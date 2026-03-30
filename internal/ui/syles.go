// Copyright 2026 Erst Users
// SPDX-License-Identifier: Apache-2.0

package ui

const (
	ansiReset   = "\033[0m"
	ansiBold    = "\033[1m"
	ansiDim     = "\033[2m"
	ansiRed     = "\033[31m"
	ansiGreen   = "\033[32m"
	ansiYellow  = "\033[33m"
	ansiCyan    = "\033[36m"
	ansiMagenta = "\033[35m"
)

// NoColor disables all ANSI output when set to true.
// Useful for piped output or terminals that do not support colour.
var NoColor bool

// Colorize wraps text in the given ANSI colour sequence.
// It is a no-op when NoColor is true or style is empty.
func Colorize(text, style string) string {
	if NoColor || style == "" {
		return text
	}
	var code string
	switch style {
	case "bold":
		code = ansiBold
	case "dim":
		code = ansiDim
	case "red":
		code = ansiRed
	case "green":
		code = ansiGreen
	case "yellow":
		code = ansiYellow
	case "cyan":
		code = ansiCyan
	case "magenta":
		code = ansiMagenta
	default:
		return text
	}
	return code + text + ansiReset
}

// DiffColors maps DiffKind values to their display style names.
var DiffColors = map[string]string{
	"added":   "green",
	"removed": "red",
	"changed": "yellow",
	"same":    "dim",
}

// BorderStyle holds the characters used to draw pane borders.
type BorderStyle struct {
	Horizontal string
	Vertical   string
	Corner     string
	Cross      string
	TeeLeft    string
	TeeRight   string
}

// DefaultBorder is the ASCII border style used by all panels.
var DefaultBorder = BorderStyle{
	Horizontal: "─",
	Vertical:   "│",
	Corner:     "+",
	Cross:      "+",
	TeeLeft:    "+",
	TeeRight:   "+",
}