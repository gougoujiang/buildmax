package db

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/apierr"
	coresecret "github.com/gougoujiang/buildmax/internal/core/secret"
)

// The db Store must satisfy the domain contract.
var _ coresecret.Store = (*Store)(nil)

// secretTestTeam creates a throwaway user and returns its id and personal team
// id, registering cleanup of the user (and its team) plus any secret rows the
// test leaves behind.
func secretTestTeam(t *testing.T, s *Store, email string) (userID, teamID string) {
	t.Helper()
	ctx := context.Background()
	if existing, _ := s.UserByEmail(ctx, email); existing != nil {
		deleteTestUser(t, s, existing.ID)
	}
	u, err := s.CreateUser(ctx, email, "free_trial")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	team, err := s.GetPersonalTeamByUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetPersonalTeamByUser: %v", err)
	}
	t.Cleanup(func() {
		s.db.Where("team_id IN (SELECT id FROM team WHERE public_id = ?)", team.ID).Delete(&secretRow{})
		deleteTestUser(t, s, u.ID)
	})
	return u.ID, team.ID
}

func sealedFixture(cipher, nonce, wrapped []byte, keyID string) coresecret.Sealed {
	return coresecret.Sealed{Ciphertext: cipher, Nonce: nonce, WrappedDEK: wrapped, KeyID: keyID}
}

func TestSecretStore_CreateGetSealedRoundTrip(t *testing.T) {
	dsn := os.Getenv(config.EnvKeyBuildmaxTestDSN)
	if dsn == "" {
		t.Skip(config.EnvKeyBuildmaxTestDSN + " not set, skipping store integration test")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	userID, teamID := secretTestTeam(t, s, "secret-roundtrip@example.com")

	sealed := sealedFixture([]byte("cipher-bytes"), []byte("nonce12bytes"), []byte("wrapped-dek"), "file:root:1")
	created, err := s.CreateSecret(ctx, coresecret.CreateInput{
		TeamID:      teamID,
		Name:        "aws-prod",
		Description: "prod read-only",
		CreatedBy:   userID,
		ItemNames:   []string{"access_key_id", "secret_access_key", "region"},
		Sealed:      sealed,
	})
	if err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}
	if created.ID == "" || created.TeamID != teamID || created.CreatedBy != userID {
		t.Fatalf("created secret = %+v", created)
	}
	if created.Provider != coresecret.ProviderEmbedded || created.State != coresecret.StateActive {
		t.Fatalf("provider/state = %q/%q", created.Provider, created.State)
	}
	if len(created.ItemNames) != 3 {
		t.Fatalf("item names = %v", created.ItemNames)
	}

	// GetSecret carries metadata only -- the type has no sealed field, so this
	// asserts the read path returns item names without touching ciphertext.
	got, err := s.GetSecret(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if got.Name != "aws-prod" || len(got.ItemNames) != 3 {
		t.Fatalf("GetSecret = %+v", got)
	}

	// GetSealed returns the exact sealed bytes.
	_, gotSealed, err := s.GetSealed(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetSealed: %v", err)
	}
	if string(gotSealed.Ciphertext) != "cipher-bytes" || gotSealed.KeyID != "file:root:1" {
		t.Fatalf("GetSealed = %+v", gotSealed)
	}
}

func TestSecretStore_UpdateItemsRewritesWhole(t *testing.T) {
	dsn := os.Getenv(config.EnvKeyBuildmaxTestDSN)
	if dsn == "" {
		t.Skip(config.EnvKeyBuildmaxTestDSN + " not set, skipping store integration test")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	userID, teamID := secretTestTeam(t, s, "secret-update@example.com")

	created, err := s.CreateSecret(ctx, coresecret.CreateInput{
		TeamID: teamID, Name: "gh", CreatedBy: userID,
		ItemNames: []string{"token"},
		Sealed:    sealedFixture([]byte("v1"), []byte("n1"), []byte("w1"), "file:root:1"),
	})
	if err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}

	if _, err := s.UpdateItems(ctx, coresecret.UpdateItemsInput{
		ID:        created.ID,
		ItemNames: []string{"token", "host"},
		Sealed:    sealedFixture([]byte("v2"), []byte("n2"), []byte("w2"), "file:root:2"),
	}); err != nil {
		t.Fatalf("UpdateItems: %v", err)
	}

	got, sealed, err := s.GetSealed(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetSealed: %v", err)
	}
	if len(got.ItemNames) != 2 || string(sealed.Ciphertext) != "v2" || sealed.KeyID != "file:root:2" {
		t.Fatalf("after update: names=%v sealed=%+v", got.ItemNames, sealed)
	}
}

func TestSecretStore_DestroyClearsMaterial(t *testing.T) {
	dsn := os.Getenv(config.EnvKeyBuildmaxTestDSN)
	if dsn == "" {
		t.Skip(config.EnvKeyBuildmaxTestDSN + " not set, skipping store integration test")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	userID, teamID := secretTestTeam(t, s, "secret-destroy@example.com")

	created, err := s.CreateSecret(ctx, coresecret.CreateInput{
		TeamID: teamID, Name: "temp", CreatedBy: userID,
		ItemNames: []string{"token"},
		Sealed:    sealedFixture([]byte("v1"), []byte("n1"), []byte("w1"), "file:root:1"),
	})
	if err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}

	// Disable first: still readable as sealed, just not grantable (state check
	// lives above the store).
	if _, err := s.SetState(ctx, created.ID, coresecret.StateDisabled); err != nil {
		t.Fatalf("SetState disabled: %v", err)
	}
	got, err := s.GetSecret(ctx, created.ID)
	if err != nil || got.State != coresecret.StateDisabled {
		t.Fatalf("after disable: %+v err=%v", got, err)
	}

	// Destroy: the row stays for audit, but GetSealed refuses -- the material
	// is gone.
	if _, err := s.SetState(ctx, created.ID, coresecret.StateDestroyed); err != nil {
		t.Fatalf("SetState destroyed: %v", err)
	}
	if _, _, err := s.GetSealed(ctx, created.ID); !errors.Is(err, apierr.ErrNotFound) {
		t.Fatalf("GetSealed after destroy = %v, want ErrNotFound", err)
	}
	// Metadata still resolves.
	if meta, err := s.GetSecret(ctx, created.ID); err != nil || meta.State != coresecret.StateDestroyed {
		t.Fatalf("metadata after destroy: %+v err=%v", meta, err)
	}
}

func TestSecretStore_TeamScopeAndUniqueness(t *testing.T) {
	dsn := os.Getenv(config.EnvKeyBuildmaxTestDSN)
	if dsn == "" {
		t.Skip(config.EnvKeyBuildmaxTestDSN + " not set, skipping store integration test")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	userA, teamA := secretTestTeam(t, s, "secret-scope-a@example.com")
	_, teamB := secretTestTeam(t, s, "secret-scope-b@example.com")

	mk := func(team, name string) error {
		_, err := s.CreateSecret(ctx, coresecret.CreateInput{
			TeamID: team, Name: name, CreatedBy: userA,
			ItemNames: []string{"k"},
			Sealed:    sealedFixture([]byte("c"), []byte("n"), []byte("w"), "file:root:1"),
		})
		return err
	}
	if err := mk(teamA, "dup"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if err := mk(teamA, "dup"); err == nil {
		t.Fatal("second create with same (team,name) should fail")
	}
	// Same name in another team is fine.
	if err := mk(teamB, "dup"); err != nil {
		t.Fatalf("same name in another team: %v", err)
	}

	// A listing is scoped to its team.
	listA, err := s.ListSecretsByTeam(ctx, teamA)
	if err != nil {
		t.Fatalf("ListSecretsByTeam A: %v", err)
	}
	for _, sec := range listA {
		if sec.TeamID != teamA {
			t.Fatalf("team A listing leaked %s from %s", sec.ID, sec.TeamID)
		}
	}
}
