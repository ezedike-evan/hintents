// Copyright 2026 Erst Users
// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
)

// sigWINCH is signal 28 (SIGWINCH) expressed as a raw syscall.Signal so that
// the constant compiles on Windows, where syscall.SIGWINCH is not defined.
// On Windows this signal never fires from the OS; the runtime.GOOS guard in
// ListenResize ensures we never register it there.
const sigWINCH = syscall.Signal(28)

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
	Focus  Pane

	SplitRatio float64

	ShowDiff bool

	resizeCh chan struct{}
}

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

func (l *SplitLayout) RightWidth() int {
	return l.Width - l.LeftWidth() - 1
}

// ListenResize starts a goroutine that updates Width/Height whenever the
// terminal is resized and signals the caller via the returned channel.
//
// On Unix/Linux/macOS this installs a SIGWINCH (signal 28) handler.
// On Windows SIGWINCH never fires, so the channel is returned as-is and
// the caller can poll TermSize() in the event loop to detect resizes.
func (l *SplitLayout) ListenResize() <-chan struct{} {
	if runtime.GOOS == "windows" {
		return l.resizeCh
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, sigWINCH)

	go func() {
		for range sig {
			w, h := TermSize()
			l.Width = w
			l.Height = h
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

	sb.WriteString(l.borderRow(lw, rw))
	sb.WriteByte('\n')

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

	bottom := "+" + strings.Repeat("-", lw) + "+" + strings.Repeat("-", rw) + "+"
	sb.WriteString(bottom)
	sb.WriteByte('\n')

	status := fmt.Sprintf(" [focus: %s]  %s", l.Focus, KeyHelp())
	if len(status) > l.Width {
		status = status[:l.Width]
	}
	sb.WriteString(status)

	fmt.Print(sb.String())
}

func (l *SplitLayout) borderRow(lw, rw int) string {
	leftLabel := l.fmtTitle(l.LeftTitle, l.Focus == PaneTrace, lw)
	rightLabel := l.fmtTitle(l.RightTitle, l.Focus == PaneState, rw)
	return "+" + leftLabel + "+" + rightLabel + "+"
}

func (l *SplitLayout) fmtTitle(title string, focused bool, width int) string {
	marker := ""
	if focused {
		marker = "*"
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

func (l *SplitLayout) divider() string {
	return "│"
}

func (l *SplitLayout) panePrefix(_ Pane) string {
	return ""
}

func cellAt(lines []string, row, width int) string {
	text := ""
	if row < len(lines) {
		text = lines[row]
	}
	text = strings.ReplaceAll(text, "\n", " ")
	if len(text) > width {
		return text[:width]
	}
	return text + strings.Repeat(" ", width-len(text))
}
