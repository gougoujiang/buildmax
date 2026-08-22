package artifact

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
	artifactsvc "github.com/gougoujiang/buildmax/internal/service/artifact"
)

// inlineMediaTypes is what a browser is allowed to render in place.
//
// An allowlist rather than a judgement about what looks safe: the content is
// arbitrary user files served from the API's own origin, so anything that can
// carry script — HTML, SVG, and anything unrecognised — has to leave as a
// download. PDF is deliberately absent from the first slice; a PDF viewer is an
// execution environment, and adding it is a decision worth making on its own
// rather than one inherited from "documents preview nicely".
var inlineMediaTypes = map[string]bool{
	"text/plain":     true,
	"text/markdown":  true,
	"image/png":      true,
	"image/jpeg":     true,
	"image/gif":      true,
	"image/webp":     true,
	"image/avif":     true,
	"image/bmp":      true,
	"image/x-icon":   true,
	"image/vnd.icon": true,
}

// inlineAllowed reports whether the stored media type may be rendered in place.
func inlineAllowed(mediaType string) bool {
	base, _, _ := strings.Cut(mediaType, ";")
	return inlineMediaTypes[strings.ToLower(strings.TrimSpace(base))]
}

func (h *Handler) artifactContentHandler(w http.ResponseWriter, r *http.Request) {
	rec, _, ok := h.resolve(w, r)
	if !ok {
		return
	}
	svc, ok := h.service(w)
	if !ok {
		return
	}
	body, err := svc.Open(r.Context(), rec)
	if err != nil {
		if errors.Is(err, artifactsvc.ErrNotFound) {
			httputil.WriteJSONError(w, http.StatusNotFound, "artifact content not found")
			return
		}
		if httputil.WriteServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "artifact_content", "artifact_id", rec.ArtifactID)
		return
	}
	defer func() { _ = body.Close() }()

	writeContentHeaders(w, rec)
	if _, err := io.Copy(w, body); err != nil {
		// The status line is already sent, so there is nothing to tell the
		// client. Logged because a truncated download is otherwise invisible.
		slog.Warn("artifact content stream ended early", "err", err, "artifact_id", rec.ArtifactID)
	}
}

// writeContentHeaders describes the bytes from what was stored and validated,
// never from anything the uploader declared.
func writeContentHeaders(w http.ResponseWriter, rec *model.Artifact) {
	inline := inlineAllowed(rec.MediaType)
	contentType := rec.MediaType
	disposition := "attachment"
	if inline {
		disposition = "inline"
	} else {
		// A type the browser will not render should not be announced as one it
		// might. Serving it as a byte stream removes the question.
		contentType = artifactsvc.FallbackMediaType
	}
	w.Header().Set("Content-Type", contentType)
	// Without this, a browser is free to sniff past the type just chosen.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", contentDisposition(disposition, rec.Filename))
	w.Header().Set("Content-Length", strconv.FormatInt(rec.SizeBytes, 10))
	// Content is immutable, but access to it is not: a caller removed from the
	// team must stop being able to read what their browser kept.
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
}

// contentDisposition writes both filename forms: the quoted one for clients
// that read only that, and the RFC 5987 one that survives non-ASCII names.
func contentDisposition(kind, filename string) string {
	return fmt.Sprintf("%s; filename=%q; filename*=UTF-8''%s", kind, asciiFilename(filename), url.PathEscape(filename))
}

// asciiFilename reduces a name to characters every client can read back.
func asciiFilename(filename string) string {
	var b strings.Builder
	for _, r := range filename {
		if r < 0x20 || r > 0x7e || r == '"' || r == '\\' {
			b.WriteByte('_')
			continue
		}
		b.WriteRune(r)
	}
	if b.Len() == 0 {
		return "download"
	}
	return b.String()
}
