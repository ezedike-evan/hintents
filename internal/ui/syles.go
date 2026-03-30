// Copyright 2026 Erst Users
// SPDX-License-Identifier: Apache-2.0

// Package ui — styles.go centralises all ANSI SGR colour codes used by the
// hintents terminal UI. Using named constants rather than raw escape sequences
// in each widget makes it trivial to disable colour (set NoColor = true) or
// add new styles in one place.
//
// Compatibility note: all codes here are standard ANSI SGR sequences (ISO
// 6429). They are the same sequences tcell uses internally when writing to a
// VT-compatible terminal, so output is compatible with any terminal that
// supports tcell — which is the requirement stated in issue #1010.
package ui

// NoColor disables all ANSI output when set to true.
// Useful for piped output or terminals that do not support colour.
// Mirror this flag into widgets.StatePanel with SetNoColor() as well.
var NoColor bool

// ANSI SGR codes
const (
	ansiReset = "\033[0m"

	ansiBold = "\033[1m"
	ansiDim  = "\033[2m"

	ansiRed     = "\033[31m"
	ansiGreen   = "\033[32m"
	ansiYellow  = "\033[33m"
	ansiBlue    = "\033[34m"
	ansiMagenta = "\033[35m"
	ansiCyan    = "\033[36m"
	ansiWhite   = "\033[37m"

	// Bold + colour — legible on both dark and light terminal themes.
	ansiBoldRed    = "\033[1;31m"
	ansiBoldGreen  = "\033[1;32m"
	ansiBoldYellow = "\033[1;33m"
	ansiBoldCyan   = "\033[1;36m"

	// Dim + colour — "before" values in changed rows.
	ansiDimRed   = "\033[2;31m"
	ansiDimGreen = "\033[2;32m"
)

var styleMap = map[string]string{
	"red":     ansiRed,
	"green":   ansiGreen,
	"yellow":  ansiYellow,
	"blue":    ansiBlue,
	"magenta": ansiMagenta,
	"cyan":    ansiCyan,
	"white":   ansiWhite,

	"bold": ansiBold,
	"dim":  ansiDim,

	"bold-red":    ansiBoldRed,
	"bold-green":  ansiBoldGreen,
	"bold-yellow": ansiBoldYellow,
	"bold-cyan":   ansiBoldCyan,

	"dim-red":   ansiDimRed,
	"dim-green": ansiDimGreen,
}

// Colorize wraps text in the ANSI sequence for style and appends a reset.
// It is a no-op when NoColor is true or the style name is not recognised.
func Colorize(text, style string) string {
	if NoColor || style == "" {
		return text
	}
	code, ok := styleMap[style]
	if !ok {
		return text
	}
	return code + text + ansiReset
}

// DiffLegend returns a compact one-line legend describing the diff colour scheme.
// Suitable as the last line of the panel or in a status bar.
func DiffLegend() string {
	if NoColor {
		return "Legend: [+] added  [-] removed  [~] changed  [ ] unchanged"
	}
	return "Legend: " +
		Colorize("[+]", "bold-green") + " added  " +
		Colorize("[-]", "bold-red") + " removed  " +
		Colorize("[~]", "bold-yellow") + " changed  " +
		Colorize("[ ]", "dim") + " unchanged"
}

// BorderStyle holds the characters used to draw pane borders.
type BorderStyle struct {
	Horizontal string
	Vertical   string
	Corner     string
}

// DefaultBorder is the ASCII border style used by all panels.
var DefaultBorder = BorderStyle{
	Horizontal: "─",
	Vertical:   "│",
	Corner:     "+",
}