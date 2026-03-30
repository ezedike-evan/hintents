// Copyright 2026 Erst Users
// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"bufio"
	"fmt"
	"os"
)

// Key represents a normalised keyboard event.
type Key int

const (
	KeyUnknown Key = iota
	KeyTab
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyEnter
	KeyQuit
	KeySlash
	KeyEscape
	KeyDiff
)

func (k Key) String() string {
	switch k {
	case KeyTab:
		return "Tab"
	case KeyUp:
		return "↑/k"
	case KeyDown:
		return "↓/j"
	case KeyLeft:
		return "←/h"
	case KeyRight:
		return "→/l"
	case KeyEnter:
		return "Enter"
	case KeyQuit:
		return "q"
	case KeySlash:
		return "/"
	case KeyEscape:
		return "Esc"
	default:
		return "?"
	}
}

func KeyHelp() string {
	return "Tab:switch-pane  ↑↓:navigate  Enter:expand  q:quit  /:search"
}

type KeyReader struct {
	r *bufio.Reader
}

func NewKeyReader() *KeyReader {
	return &KeyReader{r: bufio.NewReader(os.Stdin)}
}

func (kr *KeyReader) Read() (Key, error) {
	b, err := kr.r.ReadByte()
	if err != nil {
		return KeyUnknown, err
	}

	switch b {
	case '\t':
		return KeyTab, nil
	case '\r', '\n':
		return KeyEnter, nil
	case 'q', 'Q':
		return KeyQuit, nil
	case 'd', 'D':
		return KeyDiff, nil
	case 'k':
		return KeyUp, nil
	case 'j':
		return KeyDown, nil
	case 'h':
		return KeyLeft, nil
	case 'l':
		return KeyRight, nil
	case '/':
		return KeySlash, nil
	case 0x1b:
		return kr.readEscape()
	case 0x03:
		return KeyQuit, nil
	}
	return KeyUnknown, nil
}

func (kr *KeyReader) readEscape() (Key, error) {
	next, err := kr.r.ReadByte()
	if err != nil {
		return KeyEscape, nil
	}
	if next != '[' {
		return KeyEscape, nil
	}

	var seq []byte
	for {
		c, err := kr.r.ReadByte()
		if err != nil {
			break
		}
		seq = append(seq, c)
		if c >= 0x40 && c <= 0x7E {
			break
		}
	}

	if len(seq) == 0 {
		return KeyUnknown, nil
	}

	switch seq[len(seq)-1] {
	case 'A': // ESC[A
		return KeyUp, nil
	case 'B': // ESC[B
		return KeyDown, nil
	case 'C': // ESC[C
		return KeyRight, nil
	case 'D': // ESC[D
		return KeyLeft, nil
	}

	return KeyUnknown, nil
}

// TermSize returns the current terminal dimensions. It reads $COLUMNS and
// $LINES first, falling back to 80×24 when neither is set. The split-screen
// layout calls this on every resize signal to reflow the panes.
func TermSize() (width, height int) {
	width = readEnvInt("COLUMNS", 80)
	height = readEnvInt("LINES", 24)
	return width, height
}

func readEnvInt(name string, fallback int) int {
	val := os.Getenv(name)
	if val == "" {
		return fallback
	}
	var n int
	if _, err := fmt.Sscanf(val, "%d", &n); err == nil && n > 0 {
		return n
	}
	return fallback
}