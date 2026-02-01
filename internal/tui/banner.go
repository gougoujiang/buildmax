// Package tui provides the Bubble Tea TUI models and views.

package tui

import (
	"fmt"
	"strings"
)

// ASCII banner for "BUILDMAX" (block style, 7 lines).
const bannerArt = `
  ██████╗ ██╗   ██╗██╗██╗     ██████╗ ███╗   ███╗ █████╗ ██╗  ██╗
  ██╔══██╗██║   ██║██║██║     ██╔══██╗████╗ ████║██╔══██╗╚██╗██╔╝
  ██████╔╝██║   ██║██║██║     ██████╔╝██╔████╔██║███████║ ╚███╔╝
  ██╔══██╗██║   ██║██║██║     ██╔══██╗██║╚██╔╝██║██╔══██║ ██╔██╗
  ██████╔╝╚██████╔╝██║███████╗██████╔╝██║ ╚═╝ ██║██║  ██║██╔╝ ██╗
  ╚═════╝  ╚═════╝ ╚═╝╚══════╝╚═════╝ ╚═╝     ╚═╝╚═╝  ╚═╝╚═╝  ╚═╝
`

// bannerWithVersion returns the ASCII banner plus a version line.
func bannerWithVersion(version string) string {
	art := strings.TrimPrefix(bannerArt, "\n")
	if version != "" {
		art += fmt.Sprintf("\n  AI Agent TUI  ·  v%s\n", version)
	} else {
		art += "\n  AI Agent TUI\n"
	}
	return art
}
