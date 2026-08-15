package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/util"
)

// Project is a named local folder that groups desktop sessions.
type Project struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	FolderPath string `json:"folder_path"`
	CreatedAt  string `json:"created_at"`
	LastUsedAt string `json:"last_used_at"`
}

func projectsDir() string {
	return filepath.Join(config.DataDir(), "projects")
}

func projectsFilePath() string {
	return filepath.Join(projectsDir(), "projects.json")
}

func readProjects() ([]Project, error) {
	data, err := os.ReadFile(projectsFilePath())
	if os.IsNotExist(err) {
		return []Project{}, nil
	}
	if err != nil {
		return nil, err
	}
	var list []Project
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	if list == nil {
		list = []Project{}
	}
	return list, nil
}

func writeProjects(list []Project) error {
	if err := os.MkdirAll(projectsDir(), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(projectsFilePath(), data, 0644)
}

// touchProjectLastUsed updates last_used_at for a project in projects.json.
// Best-effort: errors are silently ignored so they don't interrupt a chat reply.
func touchProjectLastUsed(id string) {
	projects, err := readProjects()
	if err != nil {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for i := range projects {
		if projects[i].ID == id {
			projects[i].LastUsedAt = now
			_ = writeProjects(projects)
			return
		}
	}
}

func makeProject(name, folderPath string) Project {
	now := time.Now().UTC().Format(time.RFC3339)
	return Project{
		ID:         util.NewPrefixedID("p"),
		Name:       name,
		FolderPath: folderPath,
		CreatedAt:  now,
		LastUsedAt: now,
	}
}
