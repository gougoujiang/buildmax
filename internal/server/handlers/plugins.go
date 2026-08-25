package handlers

import (
	"errors"
	"fmt"
	"github.com/gougoujiang/buildmax/internal/core/apierr"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	coreplugin "github.com/gougoujiang/buildmax/internal/core/plugin"
	"github.com/gougoujiang/buildmax/internal/infra/pluginwire"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
)

// The catalog is readable by any active account. Publishing is the
// administrator's half and lives in the admin package; browsing and downloading
// are not privileged actions, because a release changes nothing until somebody
// installs it deliberately.

// listPluginsHandler serves GET /api/plugins.
func (h *Handler) listPluginsHandler(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.guard().ActiveUser(w, r); !ok {
		return
	}
	if !h.requirePluginService(w) {
		return
	}
	// Archived entries are out of the default catalog. They are still reachable
	// by name, so a link somebody saved keeps working.
	plugins, err := h.cfg.PluginService.ListEntries(r.Context(), false)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "list_plugins")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, pluginwire.CatalogResponse{Plugins: plugins})
}

// getPluginHandler serves GET /api/plugins/{plugin_name}.
func (h *Handler) getPluginHandler(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.guard().ActiveUser(w, r); !ok {
		return
	}
	if !h.requirePluginService(w) {
		return
	}
	name, ok := httputil.PathValue(w, r, "plugin_name")
	if !ok {
		return
	}
	entry, err := h.cfg.PluginService.GetEntry(r.Context(), name)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "get_plugin", "plugin_name", name)
		return
	}
	if entry == nil {
		httputil.WriteJSONError(w, http.StatusNotFound, "plugin not found")
		return
	}
	releases, err := h.cfg.PluginService.ListReleases(r.Context(), name)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "get_plugin", "plugin_name", name)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, pluginwire.PluginResponse{Plugin: *entry, Releases: releases})
}

// downloadPluginReleaseHandler serves
// GET /api/plugins/{plugin_name}/releases/{version}/download.
//
// The version is exact. Which release to install needs the client's own version
// to decide, so the client resolves it from the detail route and asks for the
// one it chose — the same path an explicit --version takes.
func (h *Handler) downloadPluginReleaseHandler(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.guard().ActiveUser(w, r); !ok {
		return
	}
	if !h.requirePluginService(w) {
		return
	}
	name, ok := httputil.PathValue(w, r, "plugin_name")
	if !ok {
		return
	}
	version, ok := httputil.PathValue(w, r, "version")
	if !ok {
		return
	}
	release, err := h.cfg.PluginService.GetRelease(r.Context(), name, version)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "download_plugin_release", "plugin_name", name)
		return
	}
	if release == nil {
		httputil.WriteJSONError(w, http.StatusNotFound, "release not found")
		return
	}
	// A withdrawn release is still downloadable, but not by accident: the
	// caller has to say it knows. Yanking is a default-selection control, not
	// a deletion, so refusing outright would strand a recovery.
	allowYanked, _ := strconv.ParseBool(r.URL.Query().Get(pluginwire.QueryAllowYanked))
	if release.Yanked() && !allowYanked {
		httputil.WriteJSONError(w, http.StatusConflict, yankedMessage(*release))
		return
	}

	body, size, err := h.cfg.PluginService.OpenPackage(r.Context(), *release)
	if err != nil {
		if errors.Is(err, apierr.ErrNotFound) {
			httputil.WriteJSONError(w, http.StatusNotFound, "the package bytes for this release are missing")
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "download_plugin_release", "plugin_name", name)
		return
	}
	defer body.Close()

	// The digest travels with the bytes so a client can verify what it received
	// without a second request, which is the whole point of publishing one.
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set(pluginwire.DigestHeader, release.Digest)
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=%q", name+"-"+version+".tar.gz"))
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, body); err != nil {
		// The status is already sent, so this can only be logged. A client that
		// verifies the digest finds the truncation for itself.
		slog.Warn("plugin download interrupted", "err", err, "plugin_name", name, "version", version)
	}
}

func yankedMessage(release coreplugin.Release) string {
	msg := "release " + release.Version + " was withdrawn"
	if release.YankedReason != "" {
		msg += ": " + release.YankedReason
	}
	return msg + ". Pass allow_yanked=true to install it anyway."
}

// requirePluginService refuses when the deployment has no Marketplace.
//
// Like the admin half, this cannot go through httputil.RequireStore: the
// service is a concrete pointer, and a typed nil is not a nil `any`.
func (h *Handler) requirePluginService(w http.ResponseWriter) bool {
	if h.cfg.PluginService == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "this deployment has no plugin marketplace")
		return false
	}
	return true
}
