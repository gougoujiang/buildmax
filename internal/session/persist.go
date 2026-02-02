// Session persist-after-reply: ensure title, save session, upsert list entry.
package session

import "time"

// PersistAfterReply runs the full "persist after reply" flow: ensure title from first user message,
// save session to dir, build list entry from session and workspace, and upsert the list entry.
// Returns the first error encountered.
func PersistAfterReply(s *Session, dir, workspace string, maxTitleLen int) error {
	EnsureTitleFromFirstUserMessage(s, maxTitleLen)
	if err := SaveToDir(s, dir); err != nil {
		return err
	}
	entry := ListEntry{
		ID:        s.ID(),
		Title:     s.Title(),
		Workspace: workspace,
		CreatedAt: s.CreatedAt().Format(time.RFC3339),
	}
	return UpsertListEntry(dir, entry)
}
