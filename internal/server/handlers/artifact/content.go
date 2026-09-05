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

	coreartifact "github.com/gougoujiang/buildmax/internal/core/artifact"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
	artifactsvc "github.com/gougoujiang/buildmax/internal/service/artifact"
)

// previewMode says how a client may show a type, not merely whether. It is the
// server's decision because the server, not the browser, is the authority on
// what is safe to render — see
// docs/design/artifact-public-sharing-and-preview.md §7.
type previewMode string

const (
	// previewInline: a renderer may show the bytes directly. Text, Markdown, and
	// the allowlisted image types — none of them execute.
	previewInline previewMode = "inline"
	// previewSandbox: an active document that must run only inside an
	// opaque-origin sandbox (a `sandbox`ed iframe, and the sandbox CSP below),
	// never in the API's or Portal's own origin. HTML is the case: a shared
	// prototype is meant to run, and this is how it runs without reaching a
	// viewer's session.
	previewSandbox previewMode = "sandbox"
	// previewNone: download only.
	previewNone previewMode = "none"
)

// inlineMediaTypes is what a browser may render in place with no sandbox.
//
// An allowlist rather than a judgement about what looks safe: none of these
// execute. HTML is handled separately (previewSandbox); SVG stays off because
// it is an active document usually embedded rather than viewed as a page, and
// an `<img>`-embedded SVG cannot get the frame sandbox. PDF is deliberately
// absent — a PDF viewer is an execution environment, a decision worth making on
// its own rather than inherited from "documents preview nicely".
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

// mediaBase reduces a stored media type to its lowercased type/subtype.
func mediaBase(mediaType string) string {
	base, _, _ := strings.Cut(mediaType, ";")
	return strings.ToLower(strings.TrimSpace(base))
}

// previewModeFor classifies a stored media type.
func previewModeFor(mediaType string) previewMode {
	base := mediaBase(mediaType)
	switch {
	case inlineMediaTypes[base]:
		return previewInline
	case base == "text/html":
		return previewSandbox
	default:
		return previewNone
	}
}

// htmlSandboxCSP forces an HTML response into a unique opaque origin — on a
// direct navigation as much as inside a frame — so a shared prototype's scripts
// cannot read cookies or localStorage or call the API as the viewer. Scripts,
// popups, and forms are allowed so a prototype works; allow-same-origin is
// never present, because with allow-scripts it would let the document remove
// its own sandbox.
const htmlSandboxCSP = "sandbox allow-scripts allow-popups allow-forms allow-modals"

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
		httputil.WriteInternalError(w, err, "handler error", "handler", "artifact_content", "artifact_id", rec.ID)
		return
	}
	defer func() { _ = body.Close() }()

	writeContentHeaders(w, rec, forceDownload(r))
	if _, err := io.Copy(w, body); err != nil {
		// The status line is already sent, so there is nothing to tell the
		// client. Logged because a truncated download is otherwise invisible.
		slog.Warn("artifact content stream ended early", "err", err, "artifact_id", rec.ID)
	}
}

// forceDownload reports whether the caller asked for a saved file rather than a
// rendered one. `?dl=1` on the content route overrides a previewable type so a
// download button and a plain link both get an attachment.
func forceDownload(r *http.Request) bool {
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("dl"))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

// writeContentHeaders describes the bytes from what was stored and validated,
// never from anything the uploader declared. download forces an attachment even
// for a previewable type.
func writeContentHeaders(w http.ResponseWriter, rec *coreartifact.Artifact, download bool) {
	mode := previewModeFor(rec.MediaType)
	if download {
		mode = previewNone
	}

	contentType := rec.MediaType
	disposition := "inline"
	switch mode {
	case previewNone:
		// A type the browser will not render should not be announced as one it
		// might. Serving it as a byte stream removes the question.
		contentType = artifactsvc.FallbackMediaType
		disposition = "attachment"
	case previewSandbox:
		// Rendered only inside an opaque origin. The header holds even on a
		// direct navigation, so the bytes can never run as this origin.
		w.Header().Set("Content-Security-Policy", htmlSandboxCSP)
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
