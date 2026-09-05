package workerclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/gougoujiang/buildmax/internal/infra/httpclient"
	"github.com/gougoujiang/buildmax/internal/tool"
)

// artifactResponse is what the server made of a published file. Only the fields
// a worker reports back to the model are decoded; the storage key is not among
// them and never leaves the server.
type artifactResponse struct {
	ID         string         `json:"id"`
	Filename   string         `json:"filename"`
	SizeBytes  int64          `json:"size_bytes"`
	Share      *artifactShare `json:"share,omitempty"`
	ShareError string         `json:"share_error,omitempty"`
}

// artifactShare is the public-link half of an upload that asked for one. The
// server builds the URLs from its public base URL; the worker only relays them.
type artifactShare struct {
	URL         string `json:"url"`
	DownloadURL string `json:"download_url"`
}

// artifactPublisher publishes a run's chosen file through the worker API.
//
// The bytes go through the server rather than straight to the object store the
// worker can already reach. That is deliberate: one code path creates
// artifacts, so the size limit, the naming rules, and the write-once ordering
// cannot come to differ between a worker and everyone else — and a worker never
// has to be told which team it is writing to.
type artifactPublisher struct {
	Cfg       WorkerAPIClientConfig
	TaskRunID string
	// ServerBaseURL renders the artifact's address for the model. Empty leaves
	// the reference as its id, which still means the same thing.
	ServerBaseURL string
}

// NewArtifactPublisher builds the run-token adapter for the artifact tool.
func NewArtifactPublisher(cfg WorkerAPIClientConfig, taskRunID, serverBaseURL string) tool.ArtifactPublisher {
	return &artifactPublisher{Cfg: cfg, TaskRunID: taskRunID, ServerBaseURL: serverBaseURL}
}

// PublishArtifact implements tool.ArtifactPublisher.
func (p *artifactPublisher) PublishArtifact(ctx context.Context, in tool.ArtifactUpload) (tool.PublishedArtifact, error) {
	endpoint := p.Cfg.BaseURL + "/api/worker/task-runs/" + url.PathEscape(p.TaskRunID) + "/artifacts"
	query := url.Values{}
	if in.Title != "" {
		query.Set("title", in.Title)
	}
	if in.Share {
		query.Set("share", "1")
	}
	if encoded := query.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	resp, err := httpclient.UploadFile(ctx, p.Cfg.Client, endpoint, p.Cfg.Token, "file", in.Path, in.Filename)
	if err != nil {
		return tool.PublishedArtifact{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		return tool.PublishedArtifact{}, httpclient.DecodeError(resp, "worker API POST "+endpoint)
	}
	var out artifactResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return tool.PublishedArtifact{}, err
	}
	published := tool.PublishedArtifact{
		ArtifactID: out.ID,
		Filename:   out.Filename,
		SizeBytes:  out.SizeBytes,
		URL:        ArtifactURL(p.ServerBaseURL, out.ID),
		ShareError: out.ShareError,
	}
	if out.Share != nil {
		published.ShareURL = out.Share.URL
		published.ShareDownloadURL = out.Share.DownloadURL
	}
	return published, nil
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
