package desktop

import (
	"context"
	"fmt"
	"strings"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/infra/git"
	tools "github.com/gougoujiang/buildmax/internal/tool"
)

// --- Models ---

type SlashModelEntry struct {
	Name          string `json:"name"`
	ProviderModel string `json:"provider_model,omitempty"`
	IsCurrent     bool   `json:"is_current"`
	// Managed and Destination say where this model sends prompts. Two entries
	// can share a display name and reach different places, so the selector has
	// to be able to tell them apart.
	Managed     bool   `json:"managed"`
	Destination string `json:"destination,omitempty"`
}

type SlashModelsResult struct {
	Current string            `json:"current"`
	Models  []SlashModelEntry `json:"models"`
}

// GetSlashModels returns configured models and the active model for a project.
func (a *App) GetSlashModels(projectID string) (SlashModelsResult, error) {
	ag, err := a.agentAppForProject(projectID)
	if err != nil {
		return SlashModelsResult{}, err
	}
	current := ag.DefaultModelName()
	configs := ag.ModelConfigs()
	models := make([]SlashModelEntry, len(configs))
	for i, c := range configs {
		models[i] = SlashModelEntry{
			Name:          c.Name,
			ProviderModel: c.ProviderModel,
			IsCurrent:     c.Name == current || c.ProviderModel == current,
			Managed:       c.IsManaged(),
			Destination:   modelDestination(c),
		}
	}
	return SlashModelsResult{Current: current, Models: models}, nil
}

// modelDestination is where a model entry sends prompts, shown next to the
// name so a managed entry is never mistaken for a local one.
func modelDestination(c agentapp.ModelConfig) string {
	if !c.IsManaged() {
		return c.BaseURL
	}
	server := strings.TrimPrefix(strings.TrimPrefix(c.ServerURL, "https://"), "http://")
	if server == "" {
		server = "no server_url"
	}
	if c.TeamID == "" {
		return server
	}
	return server + " " + c.TeamID
}

// SetProjectModel switches the active model for a project's agent.
func (a *App) SetProjectModel(projectID, modelName string) error {
	if modelName == "" {
		return fmt.Errorf("model name required")
	}
	ag, err := a.agentAppForProject(projectID)
	if err != nil {
		return err
	}
	ag.SetDefaultModel(modelName)
	return nil
}

// --- Skills ---

type SlashSkillEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
}

type SlashSkillsResult struct {
	Skills []SlashSkillEntry `json:"skills"`
}

// GetSlashSkills returns all skills discovered for the given project.
func (a *App) GetSlashSkills(projectID string) (SlashSkillsResult, error) {
	ag, err := a.agentAppForProject(projectID)
	if err != nil {
		return SlashSkillsResult{}, err
	}
	entries := ag.SkillEntries()
	out := make([]SlashSkillEntry, len(entries))
	for i, e := range entries {
		out[i] = SlashSkillEntry{Name: e.Name, Description: e.Description, Path: e.Path}
	}
	return SlashSkillsResult{Skills: out}, nil
}

// --- MCP ---

type SlashMCPServer struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	OK        bool   `json:"ok"`
	ToolCount int    `json:"tool_count"`
	Error     string `json:"error,omitempty"`
}

type SlashMCPResult struct {
	LoadError string           `json:"load_error,omitempty"`
	Servers   []SlashMCPServer `json:"servers"`
}

// GetSlashMCP returns the MCP server status for the given project.
func (a *App) GetSlashMCP(projectID string) (SlashMCPResult, error) {
	ag, err := a.agentAppForProject(projectID)
	if err != nil {
		return SlashMCPResult{}, err
	}
	status := ag.MCPStatus()
	servers := make([]SlashMCPServer, len(status.Servers))
	for i, s := range status.Servers {
		errStr := ""
		if s.Err != nil {
			errStr = s.Err.Error()
		}
		servers[i] = SlashMCPServer{
			ID:        s.ID,
			Type:      s.Type,
			OK:        s.OK,
			ToolCount: s.ToolCount,
			Error:     errStr,
		}
	}
	return SlashMCPResult{LoadError: status.LoadError, Servers: servers}, nil
}

// --- Agents ---

type SlashAgentEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsBuiltin   bool   `json:"is_builtin"`
}

type SlashAgentsResult struct {
	Agents []SlashAgentEntry `json:"agents"`
}

// GetSlashAgents returns all agent types (builtin + user-defined) for the project.
func (a *App) GetSlashAgents(projectID string) (SlashAgentsResult, error) {
	ag, err := a.agentAppForProject(projectID)
	if err != nil {
		return SlashAgentsResult{}, err
	}
	var agents []SlashAgentEntry
	for _, def := range tools.BuiltinSubAgentDefs {
		agents = append(agents, SlashAgentEntry{
			Name:        def.Name,
			Description: def.Description,
			IsBuiltin:   true,
		})
	}
	for _, def := range ag.AgentDefs() {
		agents = append(agents, SlashAgentEntry{
			Name:        def.Name,
			Description: def.Description,
			IsBuiltin:   false,
		})
	}
	return SlashAgentsResult{Agents: agents}, nil
}

// --- Git branch ---

// GetGitBranch returns the current git branch for the given project's folder,
// or an empty string if the folder is not a git repository.
func (a *App) GetGitBranch(projectID string) (string, error) {
	projects, err := readProjects()
	if err != nil {
		return "", err
	}
	for _, p := range projects {
		if p.ID == projectID {
			return git.CurrentBranch(p.FolderPath), nil
		}
	}
	return "", fmt.Errorf("project not found: %s", projectID)
}

// GetWorkspaceDiff returns the current git-backed changed-file view for a project.
func (a *App) GetWorkspaceDiff(projectID string) (git.WorkspaceDiff, error) {
	projects, err := readProjects()
	if err != nil {
		return git.WorkspaceDiff{}, err
	}
	for _, p := range projects {
		if p.ID == projectID {
			return git.ReadWorkspace(context.Background(), p.FolderPath)
		}
	}
	return git.WorkspaceDiff{}, fmt.Errorf("project not found: %s", projectID)
}
