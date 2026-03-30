// Copyright 2026 Erst Users
// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

// Pane identifies which half of the split screen currently has keyboard focus.
type Pane int

const (
	PaneTrace Pane = iota
	PaneState
	PaneDiff
)

func (p Pane) String() string {
	switch p {
	case PaneTrace:
		return "Trace"
	case PaneState:
		return "State"
	case PaneDiff:
		return "Diff"
	default:
		return "?"
	}
}

type SplitLayout struct {
	Width  int
	Height int
	Focus Pane

	LeftTitle   string
	MiddleTitle string
	RightTitle  string

	SplitRatio float64

	ShowDiff bool

	resizeCh chan struct{}
}

// NewSplitLayout creates a SplitLayout sized to the current terminal.
func NewSplitLayout() *SplitLayout {
	w, h := TermSize()
	return &SplitLayout{
		Width:       w,
		Height:      h,
		Focus:       PaneTrace,
		LeftTitle:   "Trace",
		MiddleTitle: "State",
		RightTitle:  "Diff",
		SplitRatio:  0.4,
		resizeCh:    make(chan struct{}, 1),
	}
}

func (l *SplitLayout) ToggleFocus() Pane {
	switch l.Focus {
	case PaneTrace:
		l.Focus = PaneState
	case PaneState:
		if l.ShowDiff {
			l.Focus = PaneDiff
		} else {
			l.Focus = PaneTrace
		}
	default: // PaneDiff
		l.Focus = PaneTrace
	}
	return l.Focus
}

func (l *SplitLayout) SetFocus(p Pane) {
	l.Focus = p
}

func (l *SplitLayout) ToggleDiff() bool {
	l.ShowDiff = !l.ShowDiff
	if !l.ShowDiff && l.Focus == PaneDiff {
		l.Focus = PaneState
	}
	return l.ShowDiff
}

// LeftWidth returns the number of columns for the trace (leftmost) pane.
func (l *SplitLayout) LeftWidth() int {
	ratio := l.SplitRatio
	if ratio <= 0 || ratio >= 1 {
		ratio = 0.4
	}
	w := int(float64(l.Width) * ratio)
	if w < 10 {
		w = 10
	}
	return w
}

func (l *SplitLayout) MiddleWidth() int {
	remaining := l.Width - l.LeftWidth() - 1 // –1 for left│middle divider
	if !l.ShowDiff {
		return remaining
	}
	w := remaining / 2
	if w < 8 {
		w = 8
	}
	return w
}

// RightWidth returns the number of columns for the diff pane.
// Returns 0 when ShowDiff is false.
func (l *SplitLayout) RightWidth() int {
	if !l.ShowDiff {
		return 0
	}
	remaining := l.Width - l.LeftWidth() - 1
	rw := remaining - l.MiddleWidth() - 1 // –1 for middle│right divider
	if rw < 0 {
		rw = 0
	}
	return rw
}

func (l *SplitLayout) ListenResize() <-chan struct{} {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGWINCH)

	go func() {
		for range sig {
			w, h := TermSize()
			l.Width = w
			l.Height = h
			// Non-blocking send — skip if the consumer hasn't processed the
			// previous event yet.
			select {
			case l.resizeCh <- struct{}{}:
			default:
			}
		}
	}()

	return l.resizeCh
}

func (l *SplitLayout) Render(leftLines, middleLines, rightLines []string) {
	lw := l.LeftWidth()
	mw := l.MiddleWidth()
	rw := l.RightWidth()

	contentRows := l.Height - 3
	if contentRows < 1 {
		contentRows = 1
	}

	sb := &strings.Builder{}

	// ── Top border ────────────────────────────────────────────────────────────
	sb.WriteString(l.borderRow(lw, mw, rw))
	sb.WriteByte('\n')

	// ── Content rows ─────────────────────────────────────────────────────────
	for row := 0; row < contentRows; row++ {
		sb.WriteString(cellAt(leftLines, row, lw))
		sb.WriteString("│")
		sb.WriteString(cellAt(middleLines, row, mw))
		if l.ShowDiff && rw > 0 {
			sb.WriteString("│")
			sb.WriteString(cellAt(rightLines, row, rw))
		}
		sb.WriteByte('\n')
	}

	// ── Bottom border ────────────────────────────────────────────────────────
	bottom := "+" + strings.Repeat("─", lw) + "+" + strings.Repeat("─", mw) + "+"
	if l.ShowDiff && rw > 0 {
		bottom += strings.Repeat("─", rw) + "+"
	}
	sb.WriteString(bottom)
	sb.WriteByte('\n')

	// ── Status bar ───────────────────────────────────────────────────────────
	help := KeyHelp()
	if l.ShowDiff {
		help += "  d:hide-diff"
	} else {
		help += "  d:show-diff"
	}
	status := fmt.Sprintf(" [focus: %s]  %s", l.Focus, help)
	if len(status) > l.Width {
		status = status[:l.Width]
	}
	sb.WriteString(status)

	fmt.Print(sb.String())
}

// borderRow builds the top border with centred pane titles.
func (l *SplitLayout) borderRow(lw, mw, rw int) string {
	left := l.fmtTitle(l.LeftTitle, l.Focus == PaneTrace, lw)
	middle := l.fmtTitle(l.MiddleTitle, l.Focus == PaneState, mw)
	top := "+" + left + "+" + middle + "+"
	if l.ShowDiff && rw > 0 {
		right := l.fmtTitle(l.RightTitle, l.Focus == PaneDiff, rw)
		top += right + "+"
	}
	return top
}

func (l *SplitLayout) fmtTitle(title string, focused bool, width int) string {
	marker := ""
	if focused {
		marker = "*" // simple ASCII focus marker visible in all terminals
	}
	label := fmt.Sprintf(" %s%s ", marker, title)
	pad := width - len(label)
	if pad < 0 {
		return label[:width]
	}
	left := pad / 2
	right := pad - left
	return strings.Repeat("─", left) + label + strings.Repeat("─", right)
}


// cellAt returns the display text for a specific row in a pane, padded or
// clipped to exactly width columns.
func cellAt(lines []string, row, width int) string {
	text := ""
	if row < len(lines) {
		text = lines[row]
	}
	// Strip any embedded newlines that would break the layout.
	text = strings.ReplaceAll(text, "\n", " ")

	if len(text) > width {
		return text[:width]
	}
	return text + strings.Repeat(" ", width-len(text))
}