package quota

import (
	"context"
	"errors"
	"strings"
	"testing"

	corequota "github.com/gougoujiang/buildmax/internal/core/quota"
	coreteam "github.com/gougoujiang/buildmax/internal/core/team"
)

type mockStorageReader struct {
	held int64
	err  error
}

func (m *mockStorageReader) TeamArtifactBytes(_ context.Context, _ string) (int64, error) {
	return m.held, m.err
}

// storageService builds a service whose only interesting dimension is storage.
func storageService(held int64, max int64) *Service {
	return &Service{
		TeamStore:     &mockTeamStore{team: &coreteam.Team{ID: "tm_1", QuotaTier: "pro"}},
		UsageReader:   &mockUsageReader{},
		TierStore:     &mockTierStore{tier: &corequota.Tier{TierName: "pro", MaxStorageBytes: max, PeriodDays: 30}},
		StorageReader: &mockStorageReader{held: held},
		DefaultTier:   "pro",
	}
}

func TestCheckStorageAllowsUnderTheLimit(t *testing.T) {
	allowed, reason, err := storageService(400, 1000).CheckStorage(context.Background(), "tm_1", 100)
	if err != nil {
		t.Fatalf("CheckStorage: %v", err)
	}
	if !allowed {
		t.Fatalf("refused under the limit: %s", reason)
	}
}

// The file that would cross the line is refused, not the one after it: the
// check is on what the team would hold, not on what it already holds.
func TestCheckStorageDeniesTheUploadThatWouldCross(t *testing.T) {
	allowed, reason, err := storageService(900, 1000).CheckStorage(context.Background(), "tm_1", 200)
	if err != nil {
		t.Fatalf("CheckStorage: %v", err)
	}
	if allowed {
		t.Fatal("admitted an upload that would exceed the limit")
	}
	// The numbers are in the reason because an agent reading a tool error has
	// to be able to tell "this file is too big" from "this space is full".
	if !strings.Contains(reason, "900") || !strings.Contains(reason, "1000") {
		t.Fatalf("reason = %q, want it to name what is used and what is allowed", reason)
	}
}

// Exactly at the limit is allowed; a limit is a ceiling, not a fence one short.
func TestCheckStorageAllowsExactlyTheLimit(t *testing.T) {
	allowed, _, err := storageService(900, 1000).CheckStorage(context.Background(), "tm_1", 100)
	if err != nil {
		t.Fatalf("CheckStorage: %v", err)
	}
	if !allowed {
		t.Fatal("refused an upload landing exactly on the limit")
	}
}

// A tier that sets no storage limit imposes none. This is the case every
// deployment that seeded its tiers before storage was measured is in, so
// getting it wrong would refuse uploads nobody chose to refuse.
func TestCheckStorageAllowsWhenTheTierSetsNoLimit(t *testing.T) {
	allowed, _, err := storageService(1<<40, 0).CheckStorage(context.Background(), "tm_1", 1<<20)
	if err != nil {
		t.Fatalf("CheckStorage: %v", err)
	}
	if !allowed {
		t.Fatal("a tier with no storage limit refused an upload")
	}
}

// No reader means the deployment has no artifact storage to measure. It must
// admit rather than refuse: refusing would break uploads on a deployment that
// has no quota service at all.
func TestCheckStorageAllowsWithoutAReader(t *testing.T) {
	c := storageService(0, 1000)
	c.StorageReader = nil
	allowed, _, err := c.CheckStorage(context.Background(), "tm_1", 5000)
	if err != nil {
		t.Fatalf("CheckStorage: %v", err)
	}
	if !allowed {
		t.Fatal("refused an upload with no storage reader configured")
	}
}

func TestCheckStorageAllowsAnUnknownTeamOrTier(t *testing.T) {
	noTeam := storageService(0, 1000)
	noTeam.TeamStore = &mockTeamStore{team: nil}
	if allowed, _, _ := noTeam.CheckStorage(context.Background(), "tm_x", 1); !allowed {
		t.Error("refused an upload for a team with no record")
	}

	noTier := storageService(0, 1000)
	noTier.TierStore = &mockTierStore{tier: nil}
	if allowed, _, _ := noTier.CheckStorage(context.Background(), "tm_1", 1); !allowed {
		t.Error("refused an upload for a tier that names nothing")
	}
}

// A limit that cannot be read is an error, not an allowance — the same rule the
// run and token limits follow. Answering "allowed" would mean a deployment
// whose database is unreachable accepts unmetered storage and records nothing.
func TestCheckStorageReportsAReadItCouldNotMake(t *testing.T) {
	c := storageService(0, 1000)
	c.StorageReader = &mockStorageReader{err: errors.New("database down")}
	allowed, _, err := c.CheckStorage(context.Background(), "tm_1", 1)
	if err == nil {
		t.Fatal("an unreadable storage total was not reported as an error")
	}
	if allowed {
		t.Fatal("an unreadable storage total was treated as an allowance")
	}
}

// The snapshot reports the stock alongside the rates, so Portal can say how
// full a space is without a second route.
func TestGetUsageReportsStorage(t *testing.T) {
	info, err := storageService(750, 1000).GetUsage(context.Background(), "tm_1")
	if err != nil {
		t.Fatalf("GetUsage: %v", err)
	}
	if info.StorageBytes == nil || *info.StorageBytes != 750 {
		t.Fatalf("StorageBytes = %v, want 750", info.StorageBytes)
	}
	if info.MaxStorageBytes == nil || *info.MaxStorageBytes != 1000 {
		t.Fatalf("MaxStorageBytes = %v, want 1000", info.MaxStorageBytes)
	}
}

// Absent, not zero: a deployment with no artifact storage holds no bytes in a
// way that is different from a team that has uploaded nothing.
func TestGetUsageOmitsStorageWithoutAReader(t *testing.T) {
	c := storageService(0, 1000)
	c.StorageReader = nil
	info, err := c.GetUsage(context.Background(), "tm_1")
	if err != nil {
		t.Fatalf("GetUsage: %v", err)
	}
	if info.StorageBytes != nil || info.MaxStorageBytes != nil {
		t.Fatalf("storage reported without a reader: %v / %v", info.StorageBytes, info.MaxStorageBytes)
	}
}
