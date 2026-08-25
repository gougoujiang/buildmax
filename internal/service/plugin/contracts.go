package plugin

import (
	"context"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// CatalogStore is the persistence capability used by Marketplace catalog
// orchestration. The service owns this port because it owns the transaction
// sequence and selection rules that consume it.
type CatalogStore interface {
	CreatePlugin(ctx context.Context, in model.CreatePluginInput) (*model.Plugin, error)
	GetPlugin(ctx context.Context, name string) (*model.Plugin, error)
	ListPlugins(ctx context.Context, includeArchived bool) ([]model.Plugin, error)
	UpdatePlugin(ctx context.Context, name string, in model.UpdatePluginInput) (*model.Plugin, error)
	SetPluginArchived(ctx context.Context, name string, archived bool) error
	CreatePluginRelease(ctx context.Context, in model.CreatePluginReleaseInput) (*model.PluginRelease, error)
	GetPluginRelease(ctx context.Context, name, version string) (*model.PluginRelease, error)
	ListPluginReleases(ctx context.Context, name string) ([]model.PluginRelease, error)
	YankPluginRelease(ctx context.Context, name, version, actor, reason string) error
}

// ActivationStore is the persistence capability used to curate and resolve
// the plugin releases available to a team.
type ActivationStore interface {
	ActivatePlugin(ctx context.Context, in model.ActivatePluginInput) (*model.PluginActivation, error)
	GetPluginActivation(ctx context.Context, teamID, pluginName string) (*model.PluginActivation, error)
	ListPluginActivations(ctx context.Context, teamID string) ([]model.PluginActivation, error)
	MovePluginActivationPin(ctx context.Context, in model.MovePluginActivationPinInput) (*model.PluginActivation, error)
	SetPluginActivationEnabled(ctx context.Context, teamID, pluginName string, enabled bool, actorID string) (*model.PluginActivation, error)
}
