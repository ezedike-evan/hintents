// Copyright 2026 Erst Users
// SPDX-License-Identifier: Apache-2.0

// Package widgets provides reusable terminal UI panels for the hintents
// interactive trace viewer.

package widgets

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dotandev/hintents/internal/trace"
)

// DiffKind classifies a single ledger entry change.
type DiffKind int

const (
	DiffSame    DiffKind = iota // key present in both states with the same value
	DiffAdded                   // key only in current state (new entry)
	DiffRemoved                 // key only in previous state (deleted entry)
	DiffChanged                 // key in both states but value changed
)

// String returns a short label for the diff kind used in status indicators.
func (d DiffKind) String() string {
	switch d {
	case DiffAdded:
		return "+"
	case DiffRemoved:
		return "-"
	case DiffChanged:
		return "~"
	default:
		return " "
	}
}

// Style returns the colour name for this diff kind (matches ui.Colorize keys).
func (d DiffKind) Style() string {
	switch d {
	case DiffAdded:
		return "green"
	case DiffRemoved:
		return "red"
	case DiffChanged:
		return "yellow"
	default:
		return "dim"
	}
}

// DiffEntry is a single row in the State Diff panel.
type DiffEntry struct {
	Key      string
	OldValue string // empty for DiffAdded
	NewValue string // empty for DiffRemoved
	Kind     DiffKind
}

// ComputeDiff produces the ordered list of DiffEntry values that describe
// how HostState changed between prev and curr.

func ComputeDiff(prev, curr *trace.ExecutionState) []DiffEntry {
	var oldMap, newMap map[string]interface{}
	if prev != nil {
		oldMap = prev.HostState
	}
	if curr != nil {
		newMap = curr.HostState
	}

	// Collect all keys.
	keySet := make(map[string]struct{})
	for k := range oldMap {
		keySet[k] = struct{}{}
	}
	for k := range newMap {
		keySet[k] = struct{}{}
	}

	entries := make([]DiffEntry, 0, len(keySet))
	for k := range keySet {
		oldVal, inOld := oldMap[k]
		newVal, inNew := newMap[k]

		entry := DiffEntry{Key: k}

		switch {
		case inOld && !inNew:
			entry.Kind = DiffRemoved
			entry.OldValue = formatValue(oldVal)
		case !inOld && inNew:
			entry.Kind = DiffAdded
			entry.NewValue = formatValue(newVal)
		default:
			oldStr := formatValue(oldVal)
			newStr := formatValue(newVal)
			if oldStr == newStr {
				entry.Kind = DiffSame
			} else {
				entry.Kind = DiffChanged
			}
			entry.OldValue = oldStr
			entry.NewValue = newStr
		}
		entries = append(entries, entry)
	}

	// Stable sort: changed/added/removed first, then same; alphabetical within groups.
	sort.SliceStable(entries, func(i, j int) bool {
		pi, pj := kindPriority(entries[i].Kind), kindPriority(entries[j].Kind)
		if pi != pj {
			return pi < pj
		}
		return entries[i].Key < entries[j].Key
	})

	return entries
}

func kindPriority(k DiffKind) int {
	switch k {
	case DiffChanged:
		return 0
	case DiffAdded:
		return 1
	case DiffRemoved:
		return 2
	default:
		return 3
	}
}

func formatValue(v interface{}) string {
	if v == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%v", v)
}

// ─────────────────────────────────────────────────────────────────────────────

// StatePanel is a resizable, scrollable three-column diff widget.
type StatePanel struct {
	entries     []DiffEntry
	scrollTop   int // first visible row index into entries
	selectedRow int // highlighted row index into entries
	noColor     bool
}

// NewStatePanel creates an empty StatePanel.
func NewStatePanel() *StatePanel {
	return &StatePanel{}
}

// SetStates computes a fresh diff from prev → curr and resets scroll/selection.
// Either argument may be nil (step 0 has no previous state).
func (p *StatePanel) SetStates(prev, curr *trace.ExecutionState) {
	p.entries = ComputeDiff(prev, curr)
	p.scrollTop = 0
	p.selectedRow = 0
}

// SelectUp moves the highlighted row up by one.
func (p *StatePanel) SelectUp() {
	if p.selectedRow > 0 {
		p.selectedRow--
		if p.selectedRow < p.scrollTop {
			p.scrollTop = p.selectedRow
		}
	}
}

// SelectDown moves the highlighted row down by one.
func (p *StatePanel) SelectDown(visibleRows int) {
	if p.selectedRow < len(p.entries)-1 {
		p.selectedRow++
		if p.selectedRow >= p.scrollTop+visibleRows {
			p.scrollTop = p.selectedRow - visibleRows + 1
		}
	}
}

// SelectedEntry returns the currently highlighted DiffEntry, or nil when the
// panel is empty.
func (p *StatePanel) SelectedEntry() *DiffEntry {
	if p.selectedRow < 0 || p.selectedRow >= len(p.entries) {
		return nil
	}
	e := p.entries[p.selectedRow]
	return &e
}

// Lines renders the panel into a slice of strings, each exactly width columns
// wide (padded or clipped). The first two lines are always the column headers
// and a divider. maxRows controls the total number of lines returned (including
// headers) so the caller can size the pane precisely.
//
// Colour escape sequences are included unless p.noColor is true.
func (p *StatePanel) Lines(width, maxRows int) []string {
	if width < 12 {
		width = 12
	}
	lines := make([]string, maxRows)

	// ── Column widths ────────────────────────────────────────────────────────
	// Layout: [indicator][key][│][old][│][new]
	// indicator = 2 chars ("+ ", "- ", "~ ", "  ")
	// remainder split: key 30 %, old 35 %, new 35 %
	indicatorW := 2
	rest := width - indicatorW - 2 // two '│' separators
	if rest < 9 {
		rest = 9
	}
	keyW := rest * 30 / 100
	oldW := (rest - keyW) / 2
	newW := rest - keyW - oldW

	// ── Header ───────────────────────────────────────────────────────────────
	header := fmt.Sprintf("%-*s%-*s│%-*s│%-*s",
		indicatorW, "",
		keyW, p.colorize("Key", "bold"),
		oldW, p.colorize("Old Value", "bold"),
		newW, p.colorize("New Value", "bold"),
	)
	lines[0] = clip(header, width)

	divider := strings.Repeat("─", width)
	lines[1] = divider

	// ── Data rows ────────────────────────────────────────────────────────────
	dataRows := maxRows - 2 // lines 2..maxRows-1
	if dataRows < 0 {
		dataRows = 0
	}

	for row := 0; row < dataRows; row++ {
		idx := p.scrollTop + row
		if idx >= len(p.entries) {
			lines[row+2] = strings.Repeat(" ", width)
			continue
		}
		e := p.entries[idx]
		style := e.Kind.Style()

		indicator := p.colorize(e.Kind.String()+" ", style)
		key := p.colorize(truncate(e.Key, keyW), style)
		oldVal := truncate(e.OldValue, oldW)
		newVal := truncate(e.NewValue, newW)

		// Highlight the selected row with inverted style.
		if idx == p.selectedRow {
			indicator = p.colorize("▸ ", "cyan")
			key = p.colorize(e.Key, "cyan")
			if len(key) > keyW {
				key = key[:keyW]
			}
		}

		line := fmt.Sprintf("%s%-*s│%-*s│%-*s",
			indicator,
			keyW, key,
			oldW, oldVal,
			newW, newVal,
		)
		lines[row+2] = clip(line, width)
	}

	// ── Scroll indicator (overwrites last data row when needed) ──────────────
	if len(p.entries) > dataRows && dataRows > 0 {
		total := len(p.entries)
		end := p.scrollTop + dataRows
		if end > total {
			end = total
		}
		indicator := p.colorize(
			fmt.Sprintf("  ─ %d–%d of %d entries ─", p.scrollTop+1, end, total),
			"dim",
		)
		lines[maxRows-1] = clip(indicator, width)
	}

	// ── Empty state ──────────────────────────────────────────────────────────
	if len(p.entries) == 0 && maxRows > 2 {
		lines[2] = clip(p.colorize("  (no host-state changes at this step)", "dim"), width)
	}

	return lines
}

// Summary returns a one-line count string for use in status bars.
func (p *StatePanel) Summary() string {
	added, removed, changed := 0, 0, 0
	for _, e := range p.entries {
		switch e.Kind {
		case DiffAdded:
			added++
		case DiffRemoved:
			removed++
		case DiffChanged:
			changed++
		}
	}
	return fmt.Sprintf("+%d -%d ~%d", added, removed, changed)
}

// SetNoColor disables ANSI colour output for this panel.
func (p *StatePanel) SetNoColor(v bool) {
	p.noColor = v
}

// colorize wraps text in a colour sequence unless p.noColor is set.
func (p *StatePanel) colorize(text, style string) string {
	if p.noColor {
		return text
	}
	return Colorize(text, style)
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// truncate clips s to at most n bytes, appending "…" when clipped.
func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

// clip pads or clips s to exactly n bytes (raw bytes, not runes).
func clip(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) >= n {
		return s[:n]
	}
	return s + strings.Repeat(" ", n-len(s))
}

// Colorize is re-exported so the widgets package can be used without importing
// the parent ui package, keeping the dependency graph acyclic.
func Colorize(text, style string) string {
	const reset = "\033[0m"
	codes := map[string]string{
		"bold":    "\033[1m",
		"dim":     "\033[2m",
		"red":     "\033[31m",
		"green":   "\033[32m",
		"yellow":  "\033[33m",
		"cyan":    "\033[36m",
		"magenta": "\033[35m",
	}
	code, ok := codes[style]
	if !ok || style == "" {
		return text
	}
	return code + text + reset
}