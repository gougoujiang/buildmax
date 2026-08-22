package workerclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/gougoujiang/buildmax/internal/infra/httpclient"
	"github.com/gougoujiang/buildmax/internal/tool"
)

// ArtifactResponse is what the server made of a published file. Only the fields
// a worker reports back to the model are decoded; the storage key is not among
// them and never leaves the server.
type ArtifactResponse struct {
	ArtifactID string `json:"artifact_id"`
	Filename   string `json:"filename"`
	SizeBytes  int64  `json:"size_bytes"`
}

// ArtifactPublisher publishes a run's chosen file through the worker API.
//
// The bytes go through the server rather than straight to the object store the
// worker can already reach. That is deliberate: one code path creates
// artifacts, so the size limit, the naming rules, and the write-once ordering
// cannot come to differ between a worker and everyone else — and a worker never
// has to be told which team it is writing to.
type ArtifactPublisher struct {
	Cfg       WorkerAPIClientConfig
	TaskRunID string
	// ServerBaseURL renders the artifact's address for the model. Empty leaves
	// the reference as its id, which still means the same thing.
	ServerBaseURL string
}

// PublishArtifact implements tool.ArtifactPublisher.
func (p *ArtifactPublisher) PublishArtifact(ctx context.Context, in tool.ArtifactUpload) (tool.PublishedArtifact, error) {
	endpoint := p.Cfg.BaseURL + "/api/worker/task-runs/" + url.PathEscape(p.TaskRunID) + "/artifacts"
	if in.Title != "" {
		endpoint += "?title=" + url.QueryEscape(in.Title)
	}
	resp, err := httpclient.UploadFile(ctx, p.Cfg.Client, endpoint, p.Cfg.Token, "file", in.Path, in.Filename)
	if err != nil {
		return tool.PublishedArtifact{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		return tool.PublishedArtifact{}, httpclient.DecodeError(resp, "worker API POST "+endpoint)
	}
	var out ArtifactResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return tool.PublishedArtifact{}, err
	}
	return tool.PublishedArtifact{
		ArtifactID: out.ArtifactID,
		Filename:   out.Filename,
		SizeBytes:  out.SizeBytes,
		URL:        ArtifactURL(p.ServerBaseURL, out.ArtifactID),
	}, nil
}

// ArtifactURL renders where an authorized person opens the artifact. Empty when
// the caller does not know a public base URL, which leaves the id as the whole
// reference.
func ArtifactURL(baseURL, artifactID string) string {
	if baseURL == "" || artifactID == "" {
		return ""
	}
	return baseURL + "/api/artifacts/" + url.PathEscape(artifactID)
}
