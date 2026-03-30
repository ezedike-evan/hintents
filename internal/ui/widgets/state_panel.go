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
//
// When prev is nil (e.g. at step 0) all keys in curr are treated as DiffAdded.
// When curr is nil all keys in prev are treated as DiffRemoved.
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
//
// It renders the delta of HostState between two consecutive ExecutionState
// values, using colour to highlight additions (green), removals (red), and
// modifications (yellow).
//
// Call SetStates whenever the user moves to a new step; call Lines() to
// obtain the pre-rendered string slice that SplitLayout can zip into its
// right pane.
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
// wide (padded or clipped). Layout:
//
//	line 0:         column headers (Key │ Old Value │ New Value)
//	line 1:         divider
//	lines 2..N-2:   data rows, colour-coded by DiffKind
//	line N-1:       legend or scroll indicator
//
// Colour scheme (legible on both light and dark terminals):
//
//	DiffAdded:   indicator bold-green, new value bold-green
//	DiffRemoved: indicator bold-red,   old value bold-red
//	DiffChanged: indicator bold-yellow, old value dim-red, new value bold-green
//	DiffSame:    entire row dim
//	Selected:    indicator + key overridden to bold-cyan
func (p *StatePanel) Lines(width, maxRows int) []string {
	if width < 12 {
		width = 12
	}
	lines := make([]string, maxRows)

	// ── Column widths ────────────────────────────────────────────────────────
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
	lines[1] = strings.Repeat("─", width)

	// ── Data rows ────────────────────────────────────────────────────────────
	// Reserve the last line for the legend/scroll indicator.
	dataRows := maxRows - 3 // header + divider + legend
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

		// Per-column styles for this diff kind.
		kindStyle := e.Kind.Style() // indicator + key style
		oldStyle, newStyle := e.Kind.valueStyles()

		// Selected row overrides indicator and key to bold-cyan.
		selected := idx == p.selectedRow
		if selected {
			kindStyle = "bold-cyan"
		}

		indicator := p.colorize(e.Kind.String()+" ", kindStyle)
		key := p.colorize(truncate(e.Key, keyW), kindStyle)
		oldVal := p.colorize(truncate(e.OldValue, oldW), oldStyle)
		newVal := p.colorize(truncate(e.NewValue, newW), newStyle)

		line := fmt.Sprintf("%s%-*s│%-*s│%-*s",
			indicator,
			keyW, key,
			oldW, oldVal,
			newW, newVal,
		)
		lines[row+2] = clip(line, width)
	}

	// Pad any unused data rows.
	for row := dataRows; row > 0; row-- {
		if idx := p.scrollTop + row - 1; idx >= len(p.entries) {
			lines[row+1] = strings.Repeat(" ", width)
		}
	}

	// ── Last line: scroll indicator or legend ─────────────────────────────
	lastRow := maxRows - 1
	if len(p.entries) > dataRows && dataRows > 0 {
		total := len(p.entries)
		end := p.scrollTop + dataRows
		if end > total {
			end = total
		}
		scroll := p.colorize(
			fmt.Sprintf("  ─ %d–%d of %d  ", p.scrollTop+1, end, total),
			"dim",
		)
		lines[lastRow] = clip(scroll+diffLegend(), width)
	} else {
		lines[lastRow] = clip(diffLegend(), width)
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

// diffLegend returns a compact legend line for the panel footer.
// It mirrors ui.DiffLegend() but uses the local Colorize so the widgets
// package does not need to import the parent ui package.
func diffLegend() string {
	return "Legend: " +
		Colorize("[+]", "bold-green") + " added  " +
		Colorize("[-]", "bold-red") + " removed  " +
		Colorize("[~]", "bold-yellow") + " changed  " +
		Colorize("[ ]", "dim") + " unchanged"
}

// valueStyles returns the ANSI style names to apply to the old-value and
// new-value columns for a given DiffKind.
//
// Design rationale:
//   - DiffAdded:   new value bold-green  (clearly new, nothing to compare)
//   - DiffRemoved: old value bold-red    (clearly gone, nothing to compare)
//   - DiffChanged: old value dim-red     (secondary — was), new bold-green (primary — now)
//   - DiffSame:    both dim              (visually recedes, not a change)
//
// Bold variants ensure legibility on light-background terminals where plain
// green/red can wash out against a pale background.
func (d DiffKind) valueStyles() (oldStyle, newStyle string) {
	switch d {
	case DiffAdded:
		return "", "bold-green"
	case DiffRemoved:
		return "bold-red", ""
	case DiffChanged:
		return "dim-red", "bold-green"
	default: // DiffSame
		return "dim", "dim"
	}
}

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
//
// Supported style names match those in ui/styles.go:
//
//	Plain:      red, green, yellow, cyan, magenta, blue, white
//	Intensity:  bold, dim
//	Combined:   bold-red, bold-green, bold-yellow, bold-cyan
//	            dim-red, dim-green
func Colorize(text, style string) string {
	const reset = "\033[0m"
	codes := map[string]string{
		// Plain colours
		"red":     "\033[31m",
		"green":   "\033[32m",
		"yellow":  "\033[33m",
		"blue":    "\033[34m",
		"magenta": "\033[35m",
		"cyan":    "\033[36m",
		"white":   "\033[37m",
		// Intensity
		"bold": "\033[1m",
		"dim":  "\033[2m",
		// Bold + colour (high legibility on light and dark backgrounds)
		"bold-red":    "\033[1;31m",
		"bold-green":  "\033[1;32m",
		"bold-yellow": "\033[1;33m",
		"bold-cyan":   "\033[1;36m",
		// Dim + colour ("before" / secondary values)
		"dim-red":   "\033[2;31m",
		"dim-green": "\033[2;32m",
	}
	code, ok := codes[style]
	if !ok || style == "" {
		return text
	}
	return code + text + reset
}
