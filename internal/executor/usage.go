package executor

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// UsageFilename is the well-known file name for run usage under the run global dir.
const UsageFilename = "usage.json"

// usageFile is the JSON shape written by the CLI and read by the executor.
type usageFile struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// ReadRunUsage reads usage.json from globalDir. Returns (nil, nil, nil) if the file is missing or invalid.
func ReadRunUsage(globalDir string) (promptTokens, completionTokens *int, err error) {
	path := filepath.Join(globalDir, UsageFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	var u usageFile
	if err := json.Unmarshal(data, &u); err != nil {
		return nil, nil, nil
	}
	p, c := u.PromptTokens, u.CompletionTokens
	return &p, &c, nil
}

// WriteUsageFile writes usage.json to dir with the given token counts.
func WriteUsageFile(dir string, promptTokens, completionTokens int) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, UsageFilename)
	u := usageFile{PromptTokens: promptTokens, CompletionTokens: completionTokens}
	data, err := json.MarshalIndent(u, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
