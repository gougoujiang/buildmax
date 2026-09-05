package mock

import (
	"context"
	"sort"
	"sync"
	"time"

	coreartifact "github.com/gougoujiang/buildmax/internal/core/artifact"
	"github.com/gougoujiang/buildmax/internal/util"
)

// MockArtifactShareStore is an in-memory coreartifact.ShareStore for tests. It
// resolves a share's target through the artifact store it is given, so a
// tombstoned artifact is reflected the way the real join reflects it.
type MockArtifactShareStore struct {
	mu     sync.Mutex
	shares []coreartifact.ArtifactShare
	// hashByShare maps a share's public ID to its stored token hash, and lets
	// the resolver find a share from a token hash.
	hashByShare map[string]string
	// Artifacts resolves the artifact a share points at. Required.
	Artifacts *MockArtifactStore
	// CreateErr, when set, fails every create.
	CreateErr error
}

// NewMockArtifactShareStore builds a share store backed by the given artifact
// store.
func NewMockArtifactShareStore(artifacts *MockArtifactStore) *MockArtifactShareStore {
	return &MockArtifactShareStore{hashByShare: map[string]string{}, Artifacts: artifacts}
}

func (m *MockArtifactShareStore) CreateArtifactShare(_ context.Context, in coreartifact.CreateShareInput) (*coreartifact.ArtifactShare, error) {
	if m.CreateErr != nil {
		return nil, m.CreateErr
	}
	id, err := util.NewPublicID()
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rec := coreartifact.ArtifactShare{
		ShareID:       id,
		ArtifactID:    in.ArtifactID,
		TeamID:        in.TeamID,
		CreatedByType: in.CreatedByType,
		CreatedByID:   in.CreatedByID,
		ExpiresAt:     in.ExpiresAt,
		CreatedAt:     seqTime(len(m.shares) + 1),
	}
	m.shares = append(m.shares, rec)
	m.hashByShare[id] = in.TokenSHA256
	out := rec
	return &out, nil
}

func (m *MockArtifactShareStore) GetArtifactShareByTokenHash(ctx context.Context, tokenHash string) (*coreartifact.ResolvedShare, error) {
	m.mu.Lock()
	var share *coreartifact.ArtifactShare
	for i := range m.shares {
		if m.hashByShare[m.shares[i].ShareID] == tokenHash {
			s := m.shares[i]
			share = &s
			break
		}
	}
	m.mu.Unlock()
	if share == nil {
		return nil, nil
	}
	art, err := m.Artifacts.GetArtifact(ctx, share.ArtifactID)
	if err != nil {
		return nil, err
	}
	if art == nil {
		return nil, nil
	}
	return &coreartifact.ResolvedShare{Share: *share, Artifact: *art}, nil
}

func (m *MockArtifactShareStore) ListArtifactShares(_ context.Context, artifactID string) ([]coreartifact.ArtifactShare, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []coreartifact.ArtifactShare
	for i := range m.shares {
		if m.shares[i].ArtifactID == artifactID {
			out = append(out, m.shares[i])
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (m *MockArtifactShareStore) RevokeArtifactShare(_ context.Context, artifactID, shareID string, revokedAt time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.shares {
		if m.shares[i].ShareID == shareID && m.shares[i].ArtifactID == artifactID && m.shares[i].RevokedAt == nil {
			at := revokedAt
			m.shares[i].RevokedAt = &at
			return true, nil
		}
	}
	return false, nil
}

func (m *MockArtifactShareStore) RecordArtifactShareRetrieval(_ context.Context, shareID string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.shares {
		if m.shares[i].ShareID == shareID {
			m.shares[i].RetrievalCount++
			t := at
			m.shares[i].LastRetrievedAt = &t
			return nil
		}
	}
	return nil
}
