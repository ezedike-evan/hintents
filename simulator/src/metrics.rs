// Copyright 2026 Erst Users
// SPDX-License-Identifier: Apache-2.0

//! Snapshot serialization latency metrics for the Soroban simulator.
//!
//! [`SnapshotMetrics`] accumulates per-operation timing samples collected
//! during a simulation run and produces a human-readable summary.  The summary
//! is emitted automatically at the end of execution when `--profile` or verbose
//! logging is enabled.
//!
//! # Warning threshold
//!
//! If the total time spent in snapshot operations exceeds
//! [`SLOW_THRESHOLD_PCT`] percent of the overall execution time a
//! `tracing::warn!` line is emitted.  This surfaces serialization bottlenecks
//! in production logs without requiring a separate profiling tool.
//!
//! # Usage
//!
//! ```rust,ignore
//! let mut metrics = SnapshotMetrics::new();
//!
//! let start = std::time::Instant::now();
//! let snap = LedgerSnapshot::from_base64_map(&entries)?;
//! metrics.record_take(start.elapsed());
//!
//! let start = std::time::Instant::now();
//! let _ = snap.serialize_to_base64_map()?;
//! metrics.record_serialize(start.elapsed());
//!
//! metrics.set_total_execution(total_start.elapsed());
//! metrics.emit_summary();
//! ```

use std::time::Duration;

/// Percentage of total execution time above which snapshot operations are
/// considered slow.  Emits a `tracing::warn!` when exceeded.
pub const SLOW_THRESHOLD_PCT: f64 = 30.0;

/// Per-operation timing sample set.
#[derive(Debug, Default, Clone)]
struct OpStats {
    count:    u64,
    total_ns: u64,
    min_ns:   u64,
    max_ns:   u64,
}

impl OpStats {
    fn record(&mut self, d: Duration) {
        let ns = d.as_nanos() as u64;
        self.count    += 1;
        self.total_ns += ns;
        if self.count == 1 || ns < self.min_ns {
            self.min_ns = ns;
        }
        if ns > self.max_ns {
            self.max_ns = ns;
        }
    }

    fn mean_ns(&self) -> f64 {
        if self.count == 0 {
            0.0
        } else {
            self.total_ns as f64 / self.count as f64
        }
    }
}

/// Accumulates snapshot latency samples across a single simulation run.
///
/// Create one instance per run, record samples with [`record_take`] and
/// [`record_serialize`], then call [`emit_summary`] (or
/// [`emit_summary_if_verbose`]) at the end of execution.
///
/// [`record_take`]: SnapshotMetrics::record_take
/// [`record_serialize`]: SnapshotMetrics::record_serialize
/// [`emit_summary`]: SnapshotMetrics::emit_summary
/// [`emit_summary_if_verbose`]: SnapshotMetrics::emit_summary_if_verbose
#[derive(Debug, Default, Clone)]
pub struct SnapshotMetrics {
    take:          OpStats,
    serialize:     OpStats,
    total_exec_ns: u64,
}

impl SnapshotMetrics {
    /// Creates a new, empty metrics collector.
    pub fn new() -> Self {
        Self::default()
    }

    // ── Recording ──────────────────────────────────────────────────────────

    /// Record the duration of one `take_snapshot` (i.e. `from_base64_map`)
    /// call.
    pub fn record_take(&mut self, d: Duration) {
        self.take.record(d);
    }

    /// Record the duration of one `serialize_snapshot` (i.e.
    /// `serialize_to_base64_map`) call.
    pub fn record_serialize(&mut self, d: Duration) {
        self.serialize.record(d);
    }

    /// Set the wall-clock duration of the entire simulation run.  Required for
    /// the percentage-of-total calculation in [`emit_summary`] and
    /// [`check_threshold`].
    ///
    /// Call this once, after the simulation finishes, before emitting the
    /// summary.
    ///
    /// [`emit_summary`]: SnapshotMetrics::emit_summary
    /// [`check_threshold`]: SnapshotMetrics::check_threshold
    pub fn set_total_execution(&mut self, d: Duration) {
        self.total_exec_ns = d.as_nanos() as u64;
    }

    // ── Queries ────────────────────────────────────────────────────────────

    /// Total nanoseconds spent in snapshot operations (take + serialize).
    pub fn snapshot_total_ns(&self) -> u64 {
        self.take.total_ns.saturating_add(self.serialize.total_ns)
    }

    /// Fraction of total execution time consumed by snapshot operations,
    /// expressed as a percentage.  Returns `None` when the total execution
    /// time has not been set yet or is zero.
    pub fn snapshot_pct(&self) -> Option<f64> {
        if self.total_exec_ns == 0 {
            return None;
        }
        Some(self.snapshot_total_ns() as f64 / self.total_exec_ns as f64 * 100.0)
    }

    /// Returns `true` when snapshot operations exceed [`SLOW_THRESHOLD_PCT`]
    /// percent of total execution time.
    pub fn is_slow(&self) -> bool {
        self.snapshot_pct()
            .map(|p| p > SLOW_THRESHOLD_PCT)
            .unwrap_or(false)
    }

    /// Emit a `tracing::warn!` if snapshotting consumed more than
    /// [`SLOW_THRESHOLD_PCT`] % of total execution time.
    ///
    /// Call this after [`set_total_execution`].
    ///
    /// [`set_total_execution`]: SnapshotMetrics::set_total_execution
    pub fn check_threshold(&self) {
        if let Some(pct) = self.snapshot_pct() {
            if pct > SLOW_THRESHOLD_PCT {
                tracing::warn!(
                    pct = format!("{pct:.1}"),
                    threshold_pct = SLOW_THRESHOLD_PCT,
                    snapshot_ms = self.snapshot_total_ns() / 1_000_000,
                    total_ms    = self.total_exec_ns / 1_000_000,
                    "Snapshotting consumed {pct:.1}% of total execution time \
                     (threshold: {SLOW_THRESHOLD_PCT}%). \
                     Consider reducing snapshot frequency or optimising XDR serialization."
                );
            }
        }
    }

    // ── Summary output ─────────────────────────────────────────────────────

    /// Returns a multi-line human-readable summary string.
    ///
    /// This is the string emitted by [`emit_summary`].  Expose it separately
    /// so callers can log it through their own sink (e.g. append to a report
    /// file).
    ///
    /// [`emit_summary`]: SnapshotMetrics::emit_summary
    pub fn summary(&self) -> String {
        let pct_str = match self.snapshot_pct() {
            Some(p) => format!("{p:.1}%"),
            None    => "n/a (total execution time not set)".to_string(),
        };

        let slow_tag = if self.is_slow() { " ⚠ SLOW" } else { "" };

        format!(
            "─── Snapshot Serialization Metrics{slow_tag} ───\n\
             take_snapshot:\n\
             {}\n\
             serialize_snapshot:\n\
             {}\n\
             Snapshot overhead: {} ms / total {} ms = {}",
            format_op(&self.take),
            format_op(&self.serialize),
            self.snapshot_total_ns() / 1_000_000,
            self.total_exec_ns / 1_000_000,
            pct_str,
        )
    }

    /// Print the summary to `stderr` via `tracing::info!`.
    ///
    /// Intended for use with `--profile` or verbose logging.  The caller
    /// controls whether this is invoked (e.g. gated on a CLI flag).
    pub fn emit_summary(&self) {
        tracing::info!("{}", self.summary());
    }

    /// Emit the summary only when the `verbose` flag is set.
    ///
    /// ```rust,ignore
    /// metrics.emit_summary_if_verbose(args.verbose || args.profile);
    /// ```
    pub fn emit_summary_if_verbose(&self, verbose: bool) {
        if verbose {
            self.emit_summary();
        }
        self.check_threshold(); // threshold warning always fires regardless of verbosity
    }
}

// ── Helpers ────────────────────────────────────────────────────────────────────

fn format_op(s: &OpStats) -> String {
    if s.count == 0 {
        return "  (no samples recorded)".to_string();
    }
    format!(
        "  count={count}  total={total_ms:.3}ms  \
         mean={mean_us:.1}µs  min={min_us:.1}µs  max={max_us:.1}µs",
        count    = s.count,
        total_ms = s.total_ns as f64 / 1_000_000.0,
        mean_us  = s.mean_ns()   / 1_000.0,
        min_us   = s.min_ns as f64 / 1_000.0,
        max_us   = s.max_ns as f64 / 1_000.0,
    )
}

// ── Tests ──────────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;

    fn dur_us(us: u64) -> Duration {
        Duration::from_micros(us)
    }

    #[test]
    fn test_no_samples_is_not_slow() {
        let m = SnapshotMetrics::new();
        assert!(!m.is_slow());
        assert!(m.snapshot_pct().is_none());
    }

    #[test]
    fn test_record_take_accumulates() {
        let mut m = SnapshotMetrics::new();
        m.record_take(dur_us(100));
        m.record_take(dur_us(200));
        assert_eq!(m.take.count, 2);
        assert_eq!(m.take.total_ns, 300_000);
        assert_eq!(m.take.min_ns, 100_000);
        assert_eq!(m.take.max_ns, 200_000);
    }

    #[test]
    fn test_record_serialize_accumulates() {
        let mut m = SnapshotMetrics::new();
        m.record_serialize(dur_us(50));
        assert_eq!(m.serialize.count, 1);
        assert_eq!(m.serialize.total_ns, 50_000);
    }

    #[test]
    fn test_snapshot_total_ns() {
        let mut m = SnapshotMetrics::new();
        m.record_take(dur_us(1_000));      // 1 ms
        m.record_serialize(dur_us(2_000)); // 2 ms
        assert_eq!(m.snapshot_total_ns(), 3_000_000);
    }

    #[test]
    fn test_is_slow_below_threshold() {
        let mut m = SnapshotMetrics::new();
        m.record_take(dur_us(10));  // 10 µs snapshot
        m.set_total_execution(Duration::from_millis(100)); // 100 ms total → 0.01 %
        assert!(!m.is_slow());
    }

    #[test]
    fn test_is_slow_above_threshold() {
        let mut m = SnapshotMetrics::new();
        m.record_take(Duration::from_millis(40));          // 40 ms snapshot
        m.set_total_execution(Duration::from_millis(100)); // 100 ms total → 40 %
        assert!(m.is_slow());
    }

    #[test]
    fn test_snapshot_pct_calculation() {
        let mut m = SnapshotMetrics::new();
        m.record_take(Duration::from_millis(30));
        m.set_total_execution(Duration::from_millis(100));
        let pct = m.snapshot_pct().unwrap();
        assert!((pct - 30.0).abs() < 0.1);
    }

    #[test]
    fn test_summary_contains_both_operations() {
        let mut m = SnapshotMetrics::new();
        m.record_take(dur_us(500));
        m.record_serialize(dur_us(1_500));
        m.set_total_execution(Duration::from_millis(10));
        let s = m.summary();
        assert!(s.contains("take_snapshot"));
        assert!(s.contains("serialize_snapshot"));
        assert!(s.contains("Snapshot overhead"));
    }

    #[test]
    fn test_summary_shows_slow_tag_when_above_threshold() {
        let mut m = SnapshotMetrics::new();
        m.record_take(Duration::from_millis(40));
        m.set_total_execution(Duration::from_millis(100));
        assert!(m.summary().contains("SLOW"));
    }

    #[test]
    fn test_summary_no_slow_tag_when_below_threshold() {
        let mut m = SnapshotMetrics::new();
        m.record_take(Duration::from_micros(10));
        m.set_total_execution(Duration::from_millis(100));
        assert!(!m.summary().contains("SLOW"));
    }

    #[test]
    fn test_emit_summary_if_verbose_skips_when_false() {
        // Should not panic — just verify it doesn't blow up
        let m = SnapshotMetrics::new();
        m.emit_summary_if_verbose(false);
    }
}