// Package tui provides the Bubble Tea TUI models and views.

package tui

import (
	"fmt"
	"strings"
)

// ASCII banner for "BuildMAX" (block style, 6 lines; mixed case).
const bannerArt = `
 ______        _ _     _ ______         _    _ 
(____  \      (_) |   | |  ___ \   /\  \ \  / /
 ____)  )_   _ _| | _ | | | _ | | /  \  \ \/ / 
|  __  (| | | | | |/ || | || || |/ /\ \  )  (  
| |__)  ) |_| | | ( (_| | || || | |__| |/ /\ \ 
|______/ \____|_|_|\____|_||_||_|______/_/  \_\
`

// bannerWithVersion returns the ASCII banner plus a version line.
func bannerWithVersion(version string) string {
	art := strings.TrimPrefix(bannerArt, "\n")
	if version != "" {
		art += fmt.Sprintf("\n  v%s\n", version)
	} else {
		art += "\n"
	}
	return art
}
