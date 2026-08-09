package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthz(t *testing.T) {
	s := New(Config{Addr: ":0"})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /healthz status = %d; want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q; want application/json", ct)
	}
	// Body should be {"status":"ok"}
	var out struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Status != "ok" {
		t.Errorf("status = %q; want ok", out.Status)
	}
}

func TestOpenAPI(t *testing.T) {
	s := New(Config{Addr: ":0"})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /openapi.json status = %d; want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q; want application/json", ct)
	}
	var spec map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&spec); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	paths, _ := spec["paths"].(map[string]interface{})
	if paths == nil {
		t.Fatal("openapi spec missing paths")
	}
	if _, ok := paths["/healthz"]; !ok {
		t.Error("openapi spec missing path /healthz")
	}
}

func TestSwaggerUI(t *testing.T) {
	s := New(Config{Addr: ":0"})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	for _, path := range []string{"/swagger/", "/swagger", "/swagger/index.html"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s status = %d; want 200", path, resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
			t.Errorf("GET %s Content-Type = %q; want text/html", path, ct)
		}
		_ = resp.Body.Close()
	}
}
