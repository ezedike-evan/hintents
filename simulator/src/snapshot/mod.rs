// Copyright 2026 Erst Users
// SPDX-License-Identifier: Apache-2.0

#![allow(dead_code)]

//! Ledger snapshot and storage loading utilities for Soroban simulation.

use base64::Engine;
use soroban_env_host::xdr::{LedgerEntry, LedgerKey, Limits, ReadXdr, WriteXdr};
use std::collections::HashMap;
use std::sync::Arc;

#[derive(Debug, Clone)]
pub struct LedgerSnapshot {
    base: Arc<HashMap<Vec<u8>, LedgerEntry>>,
    delta: HashMap<Vec<u8>, Option<LedgerEntry>>,
}

impl LedgerSnapshot {
    pub fn new() -> Self {
        Self {
            base: Arc::new(HashMap::new()),
            delta: HashMap::new(),
        }
    }

    pub fn from_base64_map(entries: &HashMap<String, String>) -> Result<Self, SnapshotError> {
        let mut decoded_entries = HashMap::new();

        for (key_xdr, entry_xdr) in entries {
            let key = decode_ledger_key(key_xdr)?;
            let entry = decode_ledger_entry(entry_xdr)?;

            let key_bytes = key
                .to_xdr(Limits::none())
                .map_err(|e| SnapshotError::XdrEncoding(format!("Failed to encode key: {e}")))?;

            decoded_entries.insert(key_bytes, entry);
        }

        Ok(Self {
            base: Arc::new(decoded_entries),
            delta: HashMap::new(),
        })
    }

    /// Serializes all live entries back to a `HashMap<String, String>` where
    /// both key and value are base64-encoded XDR.
    ///
    /// This is the inverse of [`from_base64_map`] and is used to measure the
    /// cost of the full serialization round-trip (the `serialize_snapshot`
    /// operation tracked by [`crate::metrics::SnapshotMetrics`]).
    ///
    /// [`from_base64_map`]: LedgerSnapshot::from_base64_map
    pub fn serialize_to_base64_map(&self) -> Result<HashMap<String, String>, SnapshotError> {
        use base64::engine::general_purpose::STANDARD;

        let mut out = HashMap::new();

        for (key_bytes, entry) in self.iter() {
            let entry_bytes = entry.to_xdr(Limits::none()).map_err(|e| {
                SnapshotError::XdrEncoding(format!("Failed to encode ledger entry: {e}"))
            })?;

            out.insert(STANDARD.encode(key_bytes), STANDARD.encode(&entry_bytes));
        }

        Ok(out)
    }

    #[allow(dead_code)]
    pub fn fork(&self) -> Self {
        Self {
            base: Arc::clone(&self.base),
            delta: HashMap::new(),
        }
    }

    pub fn len(&self) -> usize {
        let mut count = self.base.len();
        for (key, val) in &self.delta {
            match val {
                Some(_) => {
                    if !self.base.contains_key(key) {
                        count += 1;
                    }
                }
                None => {
                    if self.base.contains_key(key) {
                        count -= 1;
                    }
                }
            }
        }
        count
    }

    #[allow(dead_code)]
    pub fn is_empty(&self) -> bool {
        self.len() == 0
    }

    #[allow(dead_code)]
    pub fn iter(&self) -> impl Iterator<Item = (&Vec<u8>, &LedgerEntry)> {
        let mut entries: Vec<(&Vec<u8>, &LedgerEntry)> = Vec::new();

        for (k, v) in self.base.iter() {
            if !self.delta.contains_key(k) {
                entries.push((k, v));
            }
        }

        for (k, v) in self.delta.iter() {
            if let Some(entry) = v {
                entries.push((k, entry));
            }
        }

        entries.into_iter()
    }

    #[allow(dead_code)]
    pub fn insert(&mut self, key: Vec<u8>, entry: LedgerEntry) {
        self.delta.insert(key, Some(entry));
    }

    #[allow(dead_code)]
    pub fn get(&self, key: &[u8]) -> Option<&LedgerEntry> {
        match self.delta.get(key) {
            Some(Some(entry)) => Some(entry),
            Some(None) => None,
            None => self.base.get(key),
        }
    }
}

impl Default for LedgerSnapshot {
    fn default() -> Self {
        Self::new()
    }
}

#[derive(Debug, Clone)]
pub struct StateDiff {
    pub inserted: Vec<Vec<u8>>,
    pub modified: Vec<Vec<u8>>,
    pub deleted: Vec<Vec<u8>>,
}

pub fn diff_snapshots(before: &LedgerSnapshot, after: &LedgerSnapshot) -> StateDiff {
    let mut inserted = Vec::new();
    let mut modified = Vec::new();
    let mut deleted = Vec::new();

    for (key, after_entry) in after.iter() {
        match before.get(key) {
            None => inserted.push(key.clone()),
            Some(before_entry) => {
                let before_bytes = before_entry.to_xdr(Limits::none()).ok();
                let after_bytes = after_entry.to_xdr(Limits::none()).ok();
                if before_bytes != after_bytes {
                    modified.push(key.clone());
                }
            }
        }
    }

    for (key, _) in before.iter() {
        if after.get(key).is_none() {
            deleted.push(key.clone());
        }
    }

    inserted.sort_unstable();
    modified.sort_unstable();
    deleted.sort_unstable();

    StateDiff {
        inserted,
        modified,
        deleted,
    }
}

#[derive(Debug, thiserror::Error)]
pub enum SnapshotError {
    #[error("Failed to decode base64: {0}")]
    Base64Decode(String),

    #[error("Failed to parse XDR: {0}")]
    XdrParse(String),

    #[error("Failed to encode XDR: {0}")]
    XdrEncoding(String),

    #[error("Storage operation failed: {0}")]
    #[allow(dead_code)]
    StorageError(String),
}

pub fn decode_ledger_key(key_xdr: &str) -> Result<LedgerKey, SnapshotError> {
    if key_xdr.is_empty() {
        return Err(SnapshotError::Base64Decode(
            "LedgerKey: empty payload".to_string(),
        ));
    }

    let bytes = base64::engine::general_purpose::STANDARD
        .decode(key_xdr)
        .map_err(|e| SnapshotError::Base64Decode(format!("LedgerKey: {e}")))?;

    if bytes.is_empty() {
        return Err(SnapshotError::Base64Decode(
            "LedgerKey: decoded payload is empty".to_string(),
        ));
    }

    LedgerKey::from_xdr(bytes, Limits::none())
        .map_err(|e| SnapshotError::XdrParse(format!("LedgerKey: {e}")))
}

pub fn decode_ledger_entry(entry_xdr: &str) -> Result<LedgerEntry, SnapshotError> {
    if entry_xdr.is_empty() {
        return Err(SnapshotError::Base64Decode(
            "LedgerEntry: empty payload".to_string(),
        ));
    }

    let bytes = base64::engine::general_purpose::STANDARD
        .decode(entry_xdr)
        .map_err(|e| SnapshotError::Base64Decode(format!("LedgerEntry: {e}")))?;

    if bytes.is_empty() {
        return Err(SnapshotError::Base64Decode(
            "LedgerEntry: decoded payload is empty".to_string(),
        ));
    }

    LedgerEntry::from_xdr(bytes, Limits::none())
        .map_err(|e| SnapshotError::XdrParse(format!("LedgerEntry: {e}")))
}

#[derive(Debug, Clone)]
#[allow(dead_code)]
pub struct LoadStats {
    pub loaded_count: usize,
    pub failed_count: usize,
    pub total_count: usize,
}

impl LoadStats {
    #[allow(dead_code)]
    pub fn new(loaded: usize, failed: usize, total: usize) -> Self {
        Self {
            loaded_count: loaded,
            failed_count: failed,
            total_count: total,
        }
    }

    #[allow(dead_code)]
    pub fn is_complete(&self) -> bool {
        self.failed_count == 0 && self.loaded_count == self.total_count
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_snapshot_creation() {
        let snapshot = LedgerSnapshot::new();
        assert_eq!(snapshot.len(), 0);
        assert!(snapshot.is_empty());
    }

    #[test]
    fn test_snapshot_insert_and_get() {
        let mut snapshot = LedgerSnapshot::new();
        let key = vec![1, 2, 3, 4];
        let entry = create_dummy_ledger_entry();

        snapshot.insert(key.clone(), entry.clone());
        assert_eq!(snapshot.len(), 1);
        assert!(!snapshot.is_empty());
        assert!(snapshot.get(&key).is_some());
    }

    #[test]
    fn test_snapshot_from_empty_map() {
        let entries = HashMap::new();
        let snapshot = LedgerSnapshot::from_base64_map(&entries)
            .expect("Failed to create snapshot from empty map");
        assert!(snapshot.is_empty());
    }

    #[test]
    fn test_serialize_roundtrip_empty_snapshot() {
        let snapshot = LedgerSnapshot::new();
        let serialized = snapshot
            .serialize_to_base64_map()
            .expect("Serialization of empty snapshot should succeed");
        assert!(serialized.is_empty());
    }

    #[test]
    fn test_decode_invalid_base64() {
        let result = decode_ledger_key("not-valid-base64!!!");
        assert!(result.is_err());
        assert!(matches!(
            result.unwrap_err(),
            SnapshotError::Base64Decode(_)
        ));
    }

    #[test]
    fn test_decode_empty_payloads() {
        let key_result = decode_ledger_key("");
        assert!(key_result.is_err());
        assert!(matches!(
            key_result.unwrap_err(),
            SnapshotError::Base64Decode(_)
        ));

        let entry_result = decode_ledger_entry("");
        assert!(entry_result.is_err());
        assert!(matches!(
            entry_result.unwrap_err(),
            SnapshotError::Base64Decode(_)
        ));
    }

    #[test]
    fn test_from_base64_map_with_empty_payload_returns_error() {
        let mut entries = HashMap::new();
        entries.insert(String::new(), String::new());

        let result = LedgerSnapshot::from_base64_map(&entries);
        assert!(result.is_err());
        assert!(matches!(
            result.unwrap_err(),
            SnapshotError::Base64Decode(_)
        ));
    }

    #[test]
    fn test_load_stats() {
        let stats = LoadStats::new(10, 0, 10);
        assert!(stats.is_complete());

        let stats_with_failures = LoadStats::new(8, 2, 10);
        assert!(!stats_with_failures.is_complete());
    }

    fn create_dummy_ledger_entry() -> LedgerEntry {
        use soroban_env_host::xdr::{
            AccountEntry, AccountId, LedgerEntryData, PublicKey, SequenceNumber, Thresholds,
            Uint256,
        };

        let account_id = AccountId(PublicKey::PublicKeyTypeEd25519(Uint256([0u8; 32])));
        let account_entry = AccountEntry {
            account_id,
            balance: 1000,
            seq_num: SequenceNumber(1),
            num_sub_entries: 0,
            inflation_dest: None,
            flags: 0,
            home_domain: Default::default(),
            thresholds: Thresholds([1, 0, 0, 0]),
            signers: Default::default(),
            ext: Default::default(),
        };

        LedgerEntry {
            last_modified_ledger_seq: 1,
            data: LedgerEntryData::Account(account_entry),
            ext: Default::default(),
        }
    }
}
