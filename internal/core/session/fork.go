package session

// ForkPrefix returns the branch ending at throughID as a journal of its own.
//
// It is the branch, not the physical prefix: a parent that was rewound holds
// abandoned records too, and a child that copied those would carry history its
// own parent chain never reaches. Branch already answers this — every item it
// returns has its parent in the same slice — so the result is self-contained
// and needs no repair.
//
// Item ids are preserved, because they are the stable identity §8.3 keeps, and
// preserving them is what lets a child's records be recognised as the same work
// the parent did. Sequence numbers are not: seq is a record's physical position
// in the journal holding it, and carrying a parent's numbering across would
// make it describe a file this journal is not. The child is renumbered from
// one, which is also why a gap in the parent leaves no trace here.
func ForkPrefix(items []Item, throughID string) ([]Item, error) {
	branch, err := Branch(items, throughID)
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(branch))
	for i, it := range branch {
		it.Seq = uint64(i + 1)
		out = append(out, it)
	}
	return out, nil
}
