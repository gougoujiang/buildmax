package agent

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaxUserAuthoredSystemPromptChars bounds the Space and Agent instruction
// layers together. They are sent in full on every model call and have no
// trimming path.
const MaxUserAuthoredSystemPromptChars = 8192

// ValidateInstructionLayers applies the shared permanent-prompt budget.
func ValidateInstructionLayers(spaceInstructions, agentInstructions string) error {
	n := utf8.RuneCountInString(strings.TrimSpace(spaceInstructions)) +
		utf8.RuneCountInString(strings.TrimSpace(agentInstructions))
	if n > MaxUserAuthoredSystemPromptChars {
		return fmt.Errorf("system-prompt instructions are %d characters, limit is %d: they are sent with every model call and cannot be trimmed",
			n, MaxUserAuthoredSystemPromptChars)
	}
	return nil
}
