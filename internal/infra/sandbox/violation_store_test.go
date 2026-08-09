package sandbox

import (
	"testing"
)

func TestViolationStore_RingBuffer(t *testing.T) {
	s := NewViolationStore(3)
	for i := 0; i < 5; i++ {
		s.Add(Violation{Kind: ViolationNetDeny, Host: hostFor(i)})
	}
	got := s.Recent(0)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	// Oldest→newest: entries 2, 3, 4.
	want := []string{"h2", "h3", "h4"}
	for i, v := range got {
		if v.Host != want[i] {
			t.Errorf("entry %d: host = %q, want %q", i, v.Host, want[i])
		}
	}
	if s.Totals()[ViolationNetDeny] != 5 {
		t.Errorf("totals = %v, want net_deny=5", s.Totals())
	}
}

func TestViolationStore_RecentN(t *testing.T) {
	s := NewViolationStore(10)
	for i := 0; i < 5; i++ {
		s.Add(Violation{Kind: ViolationNetAllow, Host: hostFor(i)})
	}
	got := s.Recent(2)
	if len(got) != 2 || got[0].Host != "h3" || got[1].Host != "h4" {
		t.Errorf("Recent(2) = %+v, want last two entries h3,h4", got)
	}
}

func TestIgnoreFilter_Suppresses(t *testing.T) {
	store := NewViolationStore(10)
	filter := NewIgnoreFilter(store, func(tool string) []string {
		if tool == "Bash" {
			return []string{"net_deny:noisy.example"}
		}
		return nil
	})
	filter.Record(Violation{Kind: ViolationNetDeny, Tool: "Bash", Host: "noisy.example"})
	filter.Record(Violation{Kind: ViolationNetDeny, Tool: "Bash", Host: "loud.example"})

	all := store.Recent(0)
	if len(all) != 2 {
		t.Fatalf("len = %d, want 2", len(all))
	}
	if !all[0].Suppressed {
		t.Errorf("noisy.example should be suppressed")
	}
	if all[1].Suppressed {
		t.Errorf("loud.example should NOT be suppressed")
	}
	// Totals still count both — only display hides suppressed entries.
	if store.Totals()[ViolationNetDeny] != 2 {
		t.Errorf("totals = %v, want net_deny=2", store.Totals())
	}
}

func TestIgnoreFilter_PrefixWildcard(t *testing.T) {
	store := NewViolationStore(10)
	filter := NewIgnoreFilter(store, func(_ string) []string {
		return []string{"net_deny:*"}
	})
	filter.Record(Violation{Kind: ViolationNetDeny, Host: "any.example"})
	all := store.Recent(0)
	if !all[0].Suppressed {
		t.Errorf("prefix wildcard should suppress")
	}
}

func hostFor(i int) string {
	switch i {
	case 0:
		return "h0"
	case 1:
		return "h1"
	case 2:
		return "h2"
	case 3:
		return "h3"
	case 4:
		return "h4"
	}
	return ""
}
