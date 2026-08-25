// Package llmcatalog owns what a deployment's model catalog will accept and
// what changing it records.
//
// Two edges reach it and they are not the same authority: `buildmax-server
// model` runs at a shell with the database credentials, and /api/admin/llm is
// a signed-in System Administrator. Which one acted belongs in the trail; what
// the catalog will take does not depend on it.
//
// Adding a model stays shell-only, and that is a property of the routes rather
// than of this service: a create carries a provider credential, and doing it
// over HTTP puts one in a request body, a proxy log, and whatever the browser
// did with the form.
package llmcatalog

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/apierr"
	coreaudit "github.com/gougoujiang/buildmax/internal/core/audit"
	"github.com/gougoujiang/buildmax/internal/core/llm"
	coregw "github.com/gougoujiang/buildmax/internal/core/llmgateway"
	"github.com/gougoujiang/buildmax/internal/service/audit"
	"github.com/gougoujiang/buildmax/internal/service/llmgateway"
)

// Service is the catalog's administration workflows.
type Service struct {
	Models coregw.ModelStore
	Audit  *audit.Recorder
}

const auditTarget = "llm_model"

// ErrNameTaken is returned when a create names a model the catalog already has.
var ErrNameTaken = apierr.New(apierr.KindConflict, "a model with this name already exists")

// Validate rejects a row that could never serve a call, so a caller hears about
// it here rather than at somebody's first prompt.
func Validate(in coregw.CreateModelInput) error {
	switch {
	case in.Name == "":
		return invalidf("name", "is required")
	case in.APIURL == "":
		return invalidf("api_url", "is required")
	// A local runtime has no credential, and requiring a placeholder for it
	// would put a meaningless secret in the catalog and in the audit trail.
	case in.APIKey == "" && llm.ProviderNeedsCredential(in.ProviderType):
		return invalidf("api_key", "is required")
	case in.Model == "":
		return invalidf("model", "is required")
	case in.ContextWindow < 0:
		return invalidf("context_window", "cannot be negative")
	case in.CallTimeout < 0:
		return invalidf("call_timeout", "cannot be negative")
	case in.MaxTokens < 0:
		return invalidf("max_tokens", "cannot be negative")
	case !config.KnownReasoningEffort(in.Reasoning):
		return invalidf("reasoning", "%q is not a level; use one of %s",
			in.Reasoning, strings.Join(config.ReasoningEfforts(), ", "))
	case !llm.KnownProvider(in.ProviderType):
		return invalidf("provider_type", "%q is not implemented; use one of %s",
			in.ProviderType, strings.Join(llm.Providers(), ", "))
	case !config.KnownCacheMode(in.CacheMode):
		return invalidf("cache_mode", "%q is not a mode; use one of %s",
			in.CacheMode, strings.Join(config.CacheModes(), ", "))
	case !config.KnownCacheTTL(in.CacheTTL):
		return invalidf("cache_ttl", "%q is not a retention; use one of %s",
			in.CacheTTL, strings.Join(config.CacheTTLs(), ", "))
	}
	for _, c := range in.Capabilities {
		if !knownCapability(c) {
			return invalidf("", "unknown capability %q", c)
		}
	}
	return nil
}

func knownCapability(name string) bool {
	return llmgateway.NewCapabilitySet(llmgateway.BaselineCapabilities()...).
		Has(llmgateway.Capability(name))
}

// Create validates and adds a catalog entry.
func (s *Service) Create(ctx context.Context, in coregw.CreateModelInput, actor coreaudit.Actor) (*coregw.Model, error) {
	if err := Validate(in); err != nil {
		return nil, err
	}
	created, err := s.Models.CreateLLMModel(ctx, in)
	if err != nil {
		if errors.Is(err, coregw.ErrModelNameTaken) {
			return nil, ErrNameTaken
		}
		return nil, fmt.Errorf("create model: %w", err)
	}
	s.record(ctx, actor, coreaudit.ModelCreated, created.ID, created.Name)
	return created, nil
}

// SetEnabled turns a catalog entry on or off and returns it as it now stands.
func (s *Service) SetEnabled(ctx context.Context, modelID string, enabled bool, actor coreaudit.Actor) (*coregw.Model, error) {
	existing, err := s.Models.GetLLMModel(ctx, modelID)
	if err != nil {
		return nil, fmt.Errorf("read model: %w", err)
	}
	if existing == nil {
		return nil, apierr.New(apierr.KindNotFound, "model not found")
	}
	if err := s.Models.SetLLMModelEnabled(ctx, modelID, enabled); err != nil {
		return nil, fmt.Errorf("set model enabled: %w", err)
	}
	action := coreaudit.ModelDisabled
	if enabled {
		action = coreaudit.ModelEnabled
	}
	// The name, from both edges. The trail should distinguish a catalog change
	// by who made it, not by where -- and a reader looking at a revoked model
	// months later has the id and wants the name.
	s.record(ctx, actor, action, modelID, existing.Name)

	updated, err := s.Models.GetLLMModel(ctx, modelID)
	if err != nil || updated == nil {
		return nil, fmt.Errorf("reload model: %w", err)
	}
	return updated, nil
}

func (s *Service) record(ctx context.Context, actor coreaudit.Actor, action, modelID, detail string) {
	if s.Audit == nil {
		return
	}
	s.Audit.Record(ctx, coreaudit.Event{
		ActorType:  actor.Type,
		ActorID:    actor.ID,
		Action:     action,
		TargetType: auditTarget,
		TargetID:   modelID,
		Detail:     detail,
	})
}
