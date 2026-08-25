package plugin

import (
	"context"

	coreplugin "github.com/gougoujiang/buildmax/internal/core/plugin"
)

// CatalogStore is the persistence capability used by Marketplace catalog
// orchestration. The service owns this port because it owns the transaction
// sequence and selection rules that consume it.
type CatalogStore interface {
	CreatePlugin(ctx context.Context, in coreplugin.CreateInput) (*coreplugin.Plugin, error)
	GetPlugin(ctx context.Context, name string) (*coreplugin.Plugin, error)
	ListPlugins(ctx context.Context, includeArchived bool) ([]coreplugin.Plugin, error)
	UpdatePlugin(ctx context.Context, name string, in coreplugin.UpdateInput) (*coreplugin.Plugin, error)
	SetPluginArchived(ctx context.Context, name string, archived bool) error
	CreatePluginRelease(ctx context.Context, in coreplugin.CreateReleaseInput) (*coreplugin.Release, error)
	GetPluginRelease(ctx context.Context, name, version string) (*coreplugin.Release, error)
	ListPluginReleases(ctx context.Context, name string) ([]coreplugin.Release, error)
	YankPluginRelease(ctx context.Context, name, version, actor, reason string) error
}

// ActivationStore is the persistence capability used to curate and resolve
// the plugin releases available to a team.
type ActivationStore interface {
	ActivatePlugin(ctx context.Context, in coreplugin.ActivateInput) (*coreplugin.Activation, error)
	GetPluginActivation(ctx context.Context, teamID, pluginName string) (*coreplugin.Activation, error)
	ListPluginActivations(ctx context.Context, teamID string) ([]coreplugin.Activation, error)
	MovePluginActivationPin(ctx context.Context, in coreplugin.MovePinInput) (*coreplugin.Activation, error)
	SetPluginActivationEnabled(ctx context.Context, teamID, pluginName string, enabled bool, actorID string) (*coreplugin.Activation, error)
}
