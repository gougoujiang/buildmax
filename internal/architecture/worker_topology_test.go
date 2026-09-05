package architecture_test

// The Kubernetes side of the worker API boundary. A server.yaml that parses
// says nothing about whether the cluster actually routes worker traffic to its
// own listener and admits only worker pods to it — that lives in the Services,
// the Ingress, and the NetworkPolicy. These tests read the shipped manifests as
// resources and hold that topology to the design.
//
// See docs/design/worker-api-network-boundary.md §7 and §13.2.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/gougoujiang/buildmax/internal/infra/k8s"
)

// decodeManifest returns every YAML document in a manifest as a generic map.
func decodeManifest(t *testing.T, path string) []map[string]any {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	dec := yaml.NewDecoder(strings.NewReader(string(body)))
	var docs []map[string]any
	for {
		var doc map[string]any
		if err := dec.Decode(&doc); err != nil {
			break
		}
		if doc != nil {
			docs = append(docs, doc)
		}
	}
	if len(docs) == 0 {
		t.Fatalf("no YAML documents in %s", path)
	}
	return docs
}

func kindNamed(docs []map[string]any, kind, name string) map[string]any {
	for _, d := range docs {
		if str(d["kind"]) == kind {
			meta, _ := d["metadata"].(map[string]any)
			if meta != nil && str(meta["name"]) == name {
				return d
			}
		}
	}
	return nil
}

func str(v any) string { s, _ := v.(string); return s }

func mapOf(v any) map[string]any { m, _ := v.(map[string]any); return m }

func sliceOf(v any) []any { s, _ := v.([]any); return s }

// asInt reads a YAML scalar that may decode as int or float.
func asInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	}
	return -1
}

func TestProductionWorkerBoundaryTopology(t *testing.T) {
	docs := decodeManifest(t, filepath.Join(repoRoot(t), "deployment", "production", "buildmax.yaml"))
	assertWorkerBoundaryTopology(t, docs)
}

func TestKindWorkerBoundaryTopology(t *testing.T) {
	docs := decodeManifest(t, filepath.Join(repoRoot(t), "deployment", "buildmax-deploy.yaml"))
	assertWorkerBoundaryTopology(t, docs)
}

// assertWorkerBoundaryTopology holds a manifest to the two-Service, one-policy
// shape the boundary needs, whichever deployment it is.
func assertWorkerBoundaryTopology(t *testing.T, docs []map[string]any) {
	t.Helper()

	// The old single Service name is gone: the public one is buildmax-api, so a
	// leftover buildmax Service would be a second name for the same socket.
	if kindNamed(docs, "Service", "buildmax") != nil {
		t.Error("a Service named \"buildmax\" still exists; the public Service is buildmax-api")
	}

	// The public Service on 5678.
	pub := kindNamed(docs, "Service", "buildmax-api")
	if pub == nil {
		t.Fatal("no buildmax-api Service")
	}
	if !servicePorts(pub)[5678] {
		t.Error("buildmax-api does not expose the public port 5678")
	}

	// The internal worker Service on 5679, a ClusterIP.
	worker := kindNamed(docs, "Service", "buildmax-worker-api")
	if worker == nil {
		t.Fatal("no buildmax-worker-api Service")
	}
	if typ := str(mapOf(worker["spec"])["type"]); typ != "" && typ != "ClusterIP" {
		t.Errorf("buildmax-worker-api type = %q; the worker Service must be a ClusterIP with no external exposure", typ)
	}
	if !servicePorts(worker)[5679] {
		t.Error("buildmax-worker-api does not expose the worker port 5679")
	}

	// The server container publishes both ports.
	dep := kindNamed(docs, "Deployment", "buildmax-server")
	if dep == nil {
		t.Fatal("no buildmax-server Deployment")
	}
	ports := serverContainerPorts(t, dep)
	if !ports[5678] || !ports[5679] {
		t.Errorf("server container ports = %v; want both 5678 (public) and 5679 (worker)", ports)
	}

	// No Ingress backend points at the worker Service or the old name.
	for _, d := range docs {
		if str(d["kind"]) != "Ingress" {
			continue
		}
		for _, name := range ingressBackendServices(d) {
			if name == "buildmax-worker-api" {
				t.Error("the Ingress routes to buildmax-worker-api; the worker API must not be internet-reachable")
			}
			if name == "buildmax" {
				t.Errorf("an Ingress backend still names the old buildmax Service")
			}
		}
	}

	assertWorkerNetworkPolicy(t, docs)
}

// assertWorkerNetworkPolicy checks the policy admits the worker port only from
// pods carrying exactly the labels the Kubernetes runner stamps on worker Jobs.
func assertWorkerNetworkPolicy(t *testing.T, docs []map[string]any) {
	t.Helper()
	np := kindNamed(docs, "NetworkPolicy", "buildmax-server")
	if np == nil {
		t.Fatal("no buildmax-server NetworkPolicy; a ClusterIP without one is discoverability, not authorization")
	}
	spec := mapOf(np["spec"])
	if sel := mapOf(mapOf(spec["podSelector"])["matchLabels"]); str(sel["app"]) != "buildmax-server" {
		t.Errorf("NetworkPolicy podSelector = %v; it must select the server pods", sel)
	}

	want := k8s.WorkerPodLabels()
	var workerRuleFound bool
	for _, r := range sliceOf(spec["ingress"]) {
		rule := mapOf(r)
		var opensWorkerPort bool
		for _, p := range sliceOf(rule["ports"]) {
			if asInt(mapOf(p)["port"]) == 5679 {
				opensWorkerPort = true
			}
		}
		if !opensWorkerPort {
			continue
		}
		workerRuleFound = true
		// The worker port must be gated by a podSelector, and that selector must
		// match the labels the runner actually sets — no more, no less.
		froms := sliceOf(rule["from"])
		if len(froms) == 0 {
			t.Error("the worker-port rule has no `from`; it admits every pod in the namespace")
		}
		for _, f := range froms {
			got := mapOf(mapOf(mapOf(f)["podSelector"])["matchLabels"])
			for k, v := range want {
				if str(got[k]) != v {
					t.Errorf("worker-port selector %v does not match the runner's label %s=%s", got, k, v)
				}
			}
			if len(got) != len(want) {
				t.Errorf("worker-port selector %v has extra labels beyond the runner's %v", got, want)
			}
		}
	}
	if !workerRuleFound {
		t.Error("no NetworkPolicy ingress rule opens the worker port 5679")
	}
}

// servicePorts returns the set of `port` values a Service exposes.
func servicePorts(svc map[string]any) map[int]bool {
	out := map[int]bool{}
	for _, p := range sliceOf(mapOf(svc["spec"])["ports"]) {
		out[asInt(mapOf(p)["port"])] = true
	}
	return out
}

// serverContainerPorts returns the containerPort set of the "server" container.
func serverContainerPorts(t *testing.T, dep map[string]any) map[int]bool {
	t.Helper()
	out := map[int]bool{}
	spec := mapOf(mapOf(mapOf(dep["spec"])["template"])["spec"])
	for _, c := range sliceOf(spec["containers"]) {
		container := mapOf(c)
		if str(container["name"]) != "server" {
			continue
		}
		for _, p := range sliceOf(container["ports"]) {
			out[asInt(mapOf(p)["containerPort"])] = true
		}
	}
	return out
}

// ingressBackendServices returns every backend Service name an Ingress routes to.
func ingressBackendServices(ing map[string]any) []string {
	var out []string
	for _, rule := range sliceOf(mapOf(ing["spec"])["rules"]) {
		httpBlock := mapOf(mapOf(rule)["http"])
		for _, p := range sliceOf(httpBlock["paths"]) {
			svc := mapOf(mapOf(mapOf(mapOf(p)["backend"])["service"]))
			out = append(out, str(svc["name"]))
		}
	}
	return out
}
