// Package secret owns the Team Secret domain: a Team-owned group of named
// items, its lifecycle, and the store contract. It holds no cryptography and
// no persistence -- a value crosses this package only as opaque sealed bytes
// (Sealed) or, on the materialization path, as a decrypted item map the
// caller obtained from internal/infra/secret. See
// docs/design/team-secrets.md.
package secret

import (
	"context"
	"time"
)

// State is a Secret's lifecycle. Disabling refuses new run grants and
// materializations; destruction erases recoverable material once nothing
// references it and is terminal.
type State string

const (
	StateActive    State = "active"
	StateDisabled  State = "disabled"
	StateDestroyed State = "destroyed"
)

// Provider names where a Secret's items live. Only the embedded encrypted
// store exists today; external references are a later phase.
type Provider string

const (
	ProviderEmbedded Provider = "embedded"
)

// Secret is the metadata a caller may read. It never carries an item value:
// ItemNames lists the keys present so a listing and consumption validation
// work without decrypting anything.
type Secret struct {
	ID          string
	TeamID      string
	Name        string
	Description string
	Provider    Provider
	State       State
	ItemNames   []string
	CreatedBy   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Items is a Secret's decrypted key/value set. It leaves the crypto boundary
// only toward a materialization path, never toward a list or get API. An item
// name is an identifier so a whole group can become environment variables.
type Items map[string]string

// Sealed is the stored form of a Secret's items: ciphertext plus the envelope
// metadata needed to open it, and no plaintext. internal/infra/db persists it
// and the secret service seals and opens it; neither this package nor db
// performs cryptography.
type Sealed struct {
	Ciphertext []byte
	Nonce      []byte
	WrappedDEK []byte
	KeyID      string
}

// CreateInput carries one new Secret. ItemNames is the plaintext key set the
// caller sealed, stored in the clear; Sealed is the encrypted map.
type CreateInput struct {
	TeamID      string
	Name        string
	Description string
	Provider    Provider
	CreatedBy   string
	ItemNames   []string
	Sealed      Sealed
}

// UpdateItemsInput replaces a Secret's items -- an edit or a KEK rewrap. A
// rewrap passes the same ItemNames with a re-sealed blob; an edit passes both
// anew. The row is rewritten whole, which is what makes rotation atomic.
type UpdateItemsInput struct {
	ID        string
	ItemNames []string
	Sealed    Sealed
}

// Store persists Secret metadata and sealed items. GORM stays below it; above
// it, an absent Secret is apierr.ErrNotFound.
//
// A metadata read never carries ciphertext: GetSealed is separate from
// GetSecret so a listing or detail view cannot accidentally ship the sealed
// bytes.
type Store interface {
	CreateSecret(ctx context.Context, in CreateInput) (*Secret, error)
	GetSecret(ctx context.Context, id string) (*Secret, error)
	ListSecretsByTeam(ctx context.Context, teamID string) ([]Secret, error)
	// GetSealed returns metadata and the sealed items for materialization or
	// rewrap. It refuses a destroyed Secret.
	GetSealed(ctx context.Context, id string) (*Secret, *Sealed, error)
	UpdateItems(ctx context.Context, in UpdateItemsInput) (*Secret, error)
	SetState(ctx context.Context, id string, state State) (*Secret, error)
}
