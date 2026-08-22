package artifact

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
	artifactsvc "github.com/gougoujiang/buildmax/internal/service/artifact"
)

// uploadFormField is the multipart field carrying the file.
const uploadFormField = "file"

// uploadSlackBytes is what the request may exceed the file limit by: multipart
// boundaries, part headers, and the trailing epilogue.
const uploadSlackBytes int64 = 1 << 20

// maxTitleLen bounds the caller-supplied label. It is display text, and a
// durable column is not a place for an unbounded string.
const maxTitleLen = 200

func (h *Handler) uploadArtifactHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, svc, ok := h.teamCaller(w, r)
	if !ok {
		return
	}
	ReceiveUpload(w, r, svc, ReceiveInput{
		TeamID:        teamID,
		SourceType:    model.ArtifactSourceUserUpload,
		CreatedByType: model.ArtifactCreatorUser,
		CreatedByID:   userID,
	})
}

func (h *Handler) uploadToDefaultTeamHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.guard().UserAndDefaultTeam(w, r, r.URL.Query().Get("team_id"))
	if !ok {
		return
	}
	svc, ok := h.service(w)
	if !ok {
		return
	}
	ReceiveUpload(w, r, svc, ReceiveInput{
		TeamID:        teamID,
		SourceType:    model.ArtifactSourceUserUpload,
		CreatedByType: model.ArtifactCreatorUser,
		CreatedByID:   userID,
	})
}

// ReceiveInput is who the artifact belongs to and what produced it. The caller
// has already decided both; this package never infers provenance from a route.
type ReceiveInput struct {
	TeamID        string
	SourceType    string
	SourceID      string
	CreatedByType string
	CreatedByID   string
}

// ReceiveUpload streams one multipart file into the artifact service and writes
// the response.
//
// Exported because the worker API creates artifacts too. That route
// authenticates with a run token rather than a session, but it must produce the
// same object through the same service — a second upload path is how the two
// would come to disagree about limits, naming, and failure handling.
func ReceiveUpload(w http.ResponseWriter, r *http.Request, svc *artifactsvc.Service, in ReceiveInput) {
	// The service caps the file itself; this is the backstop that stops a
	// request body from being read at all past the point where it could still
	// be accepted.
	r.Body = http.MaxBytesReader(w, r.Body, svc.MaxBytes()+uploadSlackBytes)

	// Streamed rather than parsed into a form: ParseMultipartForm spools the
	// whole upload to a temp file before the service ever sees it, which pays
	// for every byte twice and puts artifact content on the server's disk on a
	// path that has nothing to do with where it is meant to be stored.
	reader, err := r.MultipartReader()
	if err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, "expected a multipart upload")
		return
	}
	part, err := filePart(reader)
	if err != nil {
		writeUploadError(w, err, "")
		return
	}
	defer func() { _ = part.Close() }()

	rec, err := svc.Create(r.Context(), artifactsvc.CreateInput{
		TeamID:        in.TeamID,
		Filename:      part.FileName(),
		Title:         title(r),
		SourceType:    in.SourceType,
		SourceID:      in.SourceID,
		CreatedByType: in.CreatedByType,
		CreatedByID:   in.CreatedByID,
		Content:       part,
	})
	if err != nil {
		writeUploadError(w, err, in.TeamID)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, ToResponse(rec))
}

var errNoFilePart = errors.New("no file part in upload")

// filePart returns the first part carrying a file, skipping any fields that
// come before it. Parts after it are not read: one artifact is one file, and
// the request is already being streamed straight into storage by then.
func filePart(reader *multipart.Reader) (*multipart.Part, error) {
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			return nil, errNoFilePart
		}
		if err != nil {
			return nil, err
		}
		if part.FormName() == uploadFormField && part.FileName() != "" {
			return part, nil
		}
		_ = part.Close()
	}
}

// title reads the label from the query string rather than a form field, so it
// does not depend on the client putting it before the file in the body — by the
// time a later part could be read, the file has already been streamed away.
func title(r *http.Request) string {
	t := strings.TrimSpace(r.URL.Query().Get("title"))
	if len(t) > maxTitleLen {
		return t[:maxTitleLen]
	}
	return t
}

func writeUploadError(w http.ResponseWriter, err error, teamID string) {
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) || errors.Is(err, artifactsvc.ErrTooLarge) {
		httputil.WriteJSONError(w, http.StatusRequestEntityTooLarge, artifactsvc.ErrTooLarge.Error())
		return
	}
	if errors.Is(err, errNoFilePart) {
		httputil.WriteJSONError(w, http.StatusBadRequest, "upload must include a file part named \""+uploadFormField+"\"")
		return
	}
	if httputil.WriteServiceError(w, err) {
		return
	}
	httputil.WriteInternalError(w, err, "handler error", "handler", "upload_artifact", "team_id", teamID)
}
