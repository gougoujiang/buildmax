package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// The Marketplace, the Admin plugin list, and a space's activation list are all
// empty until a release is published, which is a System Administrator action.
// These fixtures grant Alice that authority, publish the repository's own sample
// plugins, and activate one in the QA space, so all three Portal surfaces have
// realistic data.

// samplePlugins are published in catalog order. Each is a real directory under
// sample-plugins/, so the fixture stays in step with what the repository ships
// rather than carrying a second copy of the content.
var samplePlugins = []struct {
	name, displayName, description string
}{
	{"code-review", "Code Review", "Structured code review skills and a read-only reviewer subagent."},
	{"commit-helper", "Commit Helper", "Write clear Conventional Commits messages from a staged diff."},
	{"docs-smith", "Docs Smith", "Skills for writing task-oriented docs and checking Markdown links."},
}

// fixtureActivatedPlugin is activated in the QA space; the others stay published
// but unactivated, so the space Plugins page shows a real activation while the
// Marketplace still offers something to activate.
const fixtureActivatedPlugin = "code-review"

func seedPluginFixtures(ctx context.Context, client *http.Client, target smokeTarget, spaceID, token string) error {
	// Publishing needs System Administrator authority. Alice already owns the
	// rich fixtures, so she is the natural holder, and a QA deployment also needs
	// an account that can reach the admin surfaces. "already holds" is the
	// idempotent success here.
	if out, err := target.admin("admin", "grant", "alice@buildmax.local"); err != nil && !strings.Contains(out, "already holds") {
		return fmt.Errorf("grant sysadmin to alice@buildmax.local: %w\n%s", err, out)
	}

	root, err := moduleRoot()
	if err != nil {
		return err
	}
	for _, p := range samplePlugins {
		dir := filepath.Join(root, "sample-plugins", p.name)
		if err := ensurePluginEntry(ctx, client, target, token, p.name, p.displayName, p.description); err != nil {
			return err
		}
		created, err := publishPluginRelease(ctx, client, target, token, p.name, dir)
		if err != nil {
			return err
		}
		if created {
			fmt.Printf("  plugin %q: published\n", p.name)
		} else {
			fmt.Printf("  plugin %q: exists\n", p.name)
		}
	}
	return ensurePluginActivation(ctx, client, target, spaceID, token, fixtureActivatedPlugin)
}

// ensurePluginEntry reserves the catalog name. A name already taken is the
// idempotent case, not a failure.
func ensurePluginEntry(ctx context.Context, client *http.Client, target smokeTarget, token, name, displayName, description string) error {
	body := map[string]string{"name": name, "display_name": displayName, "description": description}
	_, err := postAllowConflict(ctx, client, target.apiBase+"/api/admin/plugins", token, "application/json", jsonReader(body))
	return err
}

// publishPluginRelease packs the directory and uploads it, reporting whether the
// version was newly published. The version comes from the directory's own
// plugin.yaml, and republishing it is refused, which is the idempotent case.
func publishPluginRelease(ctx context.Context, client *http.Client, target smokeTarget, token, name, dir string) (bool, error) {
	archive, err := packPluginDir(dir)
	if err != nil {
		return false, err
	}
	endpoint := target.apiBase + "/api/admin/plugins/" + url.PathEscape(name) + "/releases"
	return postAllowConflict(ctx, client, endpoint, token, "application/gzip", archive)
}

// ensurePluginActivation activates one plugin in the space if it is not already,
// so the space Plugins page has a real activation to render.
func ensurePluginActivation(ctx context.Context, client *http.Client, target smokeTarget, spaceID, token, name string) error {
	base := target.apiBase + "/api/spaces/" + url.PathEscape(spaceID) + "/plugin-activations"
	var current struct {
		Activations []struct {
			PluginName string `json:"plugin_name"`
		} `json:"activations"`
	}
	if err := requestJSON(ctx, client, http.MethodGet, base, token, nil, &current, http.StatusOK); err != nil {
		return err
	}
	for _, a := range current.Activations {
		if a.PluginName == name {
			return nil
		}
	}
	return requestJSON(ctx, client, http.MethodPost, base, token, map[string]string{"plugin_name": name}, nil, http.StatusCreated)
}

// postAllowConflict treats 409 as an idempotent success -- the entry or version
// already exists -- and reports whether the resource was created (201).
func postAllowConflict(ctx context.Context, client *http.Client, endpoint, token, contentType string, body io.Reader) (bool, error) {
	response, err := request(ctx, client, http.MethodPost, endpoint, token, contentType, body, http.StatusCreated)
	if err == nil {
		return true, response.Close()
	}
	if strings.Contains(err.Error(), "409") {
		return false, nil
	}
	return false, err
}

func jsonReader(v any) io.Reader {
	data, _ := json.Marshal(v)
	return bytes.NewReader(data)
}

// packPluginDir builds the plugin archive the Marketplace expects: a gzipped tar
// of the directory's regular files, keyed by their relative paths, with .git
// excluded. It mirrors internal/infra/pluginarchive, which tools/mk cannot
// import, and the server recomputes the digest from these bytes.
func packPluginDir(dir string) (io.Reader, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	root := os.DirFS(dir)
	err := fs.WalkDir(root, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		if d.IsDir() && path.Base(name) == ".git" {
			return fs.SkipDir
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return tw.WriteHeader(&tar.Header{Name: name + "/", Typeflag: tar.TypeDir, Mode: 0o755, Format: tar.FormatPAX})
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file; plugin archives carry no symlinks", name)
		}
		if err := tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644, Size: info.Size(), Format: tar.FormatPAX}); err != nil {
			return err
		}
		f, err := root.Open(name)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("pack %s: %w", dir, err)
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return &buf, nil
}
