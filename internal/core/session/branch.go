package session

import (
	"fmt"
)

// Validate checks a journal's items as a graph, in physical order.
//
// It rejects what would make a reduction wrong: a duplicate identity, a parent
// that does not exist, more than one root, and a record this build cannot
// interpret but must. It does not check that seq numbers are contiguous — see
// docs/design/local-session-storage.md §7.2 for why a gap is not treated as a
// corruption signal.
func Validate(items []Item) error {
	seen := make(map[string]int, len(items))
	roots := 0
	for i, it := range items {
		switch {
		case it.ID == "":
			return fmt.Errorf("%w: item at index %d has no id", ErrHistoryCorrupt, i)
		case it.Payload == nil:
			return fmt.Errorf("%w: item %s has no payload", ErrHistoryCorrupt, it.ID)
		}
		if _, dup := seen[it.ID]; dup {
			return fmt.Errorf("%w: duplicate item id %s", ErrHistoryCorrupt, it.ID)
		}
		if u, ok := it.Payload.(UnknownPayload); ok && it.Required {
			return fmt.Errorf("%w: %s at seq %d", ErrUnknownRequired, u.Kind, it.Seq)
		}
		if it.ParentID == "" {
			roots++
			if roots > 1 {
				return fmt.Errorf("%w: item %s starts a second root", ErrHistoryCorrupt, it.ID)
			}
		} else if _, ok := seen[it.ParentID]; !ok {
			// A parent must already have been written. Requiring it to appear
			// earlier is what makes cycles impossible without walking for them.
			return fmt.Errorf("%w: item %s names parent %s, which does not appear before it", ErrHistoryCorrupt, it.ID, it.ParentID)
		}
		seen[it.ID] = i
	}
	if len(items) > 0 && roots == 0 {
		return fmt.Errorf("%w: no root item", ErrHistoryCorrupt)
	}
	return nil
}

// Head returns the id of the item the next append extends.
//
// It is the last physical record, with no special case for rewind. Every record
// chains to its physical predecessor except HeadSelected, which chains to the
// item being returned to, so redirecting the branch is already expressed in the
// parent links and the head never has to be stored or searched for.
func Head(items []Item) (string, error) {
	if len(items) == 0 {
		return "", fmt.Errorf("%w: journal has no items", ErrHeadNotFound)
	}
	return items[len(items)-1].ID, nil
}

// Branch returns the items from the root to head in logical order.
//
// Items on abandoned branches are skipped, which is the whole point of keeping
// them: rewind leaves them in the file, and only the parent chain decides what
// the model sees.
func Branch(items []Item, head string) ([]Item, error) {
	if head == "" {
		return nil, nil
	}
	byID := make(map[string]Item, len(items))
	for _, it := range items {
		byID[it.ID] = it
	}
	var reversed []Item
	// The walk is bounded by the item count because Validate has already
	// established that every parent appears earlier, so the chain strictly
	// descends and cannot loop.
	for id := head; id != ""; {
		it, ok := byID[id]
		if !ok {
			if id == head {
				return nil, fmt.Errorf("%w: %s", ErrHeadNotFound, head)
			}
			return nil, fmt.Errorf("%w: item %s names missing parent %s", ErrHistoryCorrupt, reversed[len(reversed)-1].ID, id)
		}
		reversed = append(reversed, it)
		id = it.ParentID
		if len(reversed) > len(items) {
			return nil, fmt.Errorf("%w: parent chain from %s does not terminate", ErrHistoryCorrupt, head)
		}
	}
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	return reversed, nil
}
