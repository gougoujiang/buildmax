package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	defaultKindCluster    = "buildmaxdev"
	defaultKindPortalPort = "8080"
	defaultKindTLSPort    = "8443"

	// kind is a plain Go program, so it runs from a pin here rather than from a
	// contributor's PATH — the same treatment golangci-lint, actionlint, and the
	// rest already get. Docker and kubectl cannot be handled this way, which is
	// why they stay in requireCommands. Pinning also means one version creates,
	// inspects, and deletes a cluster; a PATH install cannot promise that.
	// The first run builds it in a few seconds, then the Go build cache serves it.
	kindPkg = "sigs.k8s.io/kind@v0.31.0"

	kindConfigPath = "deployment/kind/kind-config.yaml"
)

func runKind(args ...string) error {
	return runCmd("go", append([]string{"run", kindPkg}, args...)...)
}

func captureKind(args ...string) (string, error) {
	return captureErr("go", append([]string{"run", kindPkg}, args...)...)
}

func cmdKind(args []string) error {
	if len(args) == 0 || len(args) > 2 {
		return usageErrorf("kind", "kind needs an action")
	}
	switch args[0] {
	case "up":
		return kindUp()
	case "images":
		return cmdPubImages()
	case "seed":
		if len(args) > 1 {
			return usageErrorf("kind", "seed takes no arguments")
		}
		return kindSeed()
	case "forward":
		if len(args) > 1 {
			return usageErrorf("kind", "forward takes no arguments")
		}
		return kindForward()
	case "info":
		return kindInfo(args[1:])
	case "smoke":
		managed, err := composeSmokeMode(args[1:])
		if err != nil {
			return err
		}
		if managed {
			return kindManagedSmoke()
		}
		return kindSmoke()
	case "status":
		return kindStatus()
	case "logs":
		return kindLogs()
	case "down":
		return kindDown()
	default:
		return usageErrorf("kind", "unknown kind action: %s", args[0])
	}
}

func kindClusterName() string {
	return envOr("BUILDMAX_KIND_CLUSTER", defaultKindCluster)
}

// kindPortalPort and kindTLSPort are the host ports this cluster's ingress
// publishes. They default to the ports every doc and script assumes, so a
// second cluster only has to name both this and BUILDMAX_KIND_CLUSTER to
// exist alongside the first one without a port collision.
func kindPortalPort() string {
	return envOr("BUILDMAX_KIND_PORTAL_PORT", defaultKindPortalPort)
}

func kindTLSPort() string {
	return envOr("BUILDMAX_KIND_TLS_PORT", defaultKindTLSPort)
}

func kindPortalURL() string {
	return "http://localhost:" + kindPortalPort()
}

func kindContext() string {
	return "kind-" + kindClusterName()
}

func kindKubectl(args ...string) error {
	return runCmd("kubectl", append([]string{"--context", kindContext()}, args...)...)
}

func captureKindKubectl(args ...string) (string, error) {
	return capture("kubectl", append([]string{"--context", kindContext()}, args...)...)
}

func kindUp() error {
	if err := requireCommands("docker", "kubectl"); err != nil {
		return err
	}
	if !succeeds("docker", "info") {
		return errors.New("docker is installed but the engine is not ready")
	}

	cluster := kindClusterName()
	previousContext, _ := capture("kubectl", "config", "current-context")
	exists, err := kindClusterExists(cluster)
	if err != nil {
		return err
	}
	if !exists {
		// Before the cluster, not after: creating one is the step that publishes
		// the ports, and everything expensive comes later. A conflict found here
		// costs a second; found by the ingress, it costs an image build first.
		if err := checkKindHostPorts(cluster); err != nil {
			return err
		}
		fmt.Printf("Creating kind cluster %q on port %s...\n", cluster, kindPortalPort())
		configPath, cleanup, err := renderKindConfig()
		if err != nil {
			return err
		}
		defer cleanup()
		if err := runKind("create", "cluster", "--name", cluster, "--config", configPath); err != nil {
			return err
		}
		// kind makes the new cluster globally current. Every command below uses
		// an explicit context, so restore the contributor's previous selection.
		if previousContext != "" && previousContext != kindContext() {
			if err := runCmd("kubectl", "config", "use-context", previousContext); err != nil {
				return fmt.Errorf("restore kubectl context %q: %w", previousContext, err)
			}
		}
	} else {
		fmt.Printf("Using existing kind cluster %q.\n", cluster)
		if err := validateKindPortMapping(cluster); err != nil {
			return err
		}
	}
	if err := kindKubectl("wait", "--for=condition=Ready", "nodes", "--all", "--timeout=120s"); err != nil {
		return err
	}

	fmt.Println("Installing ingress and backing services...")
	if err := kindKubectl("apply", "-f", "deployment/kind/kind-ingress-nginx.yaml"); err != nil {
		return err
	}
	if err := waitForKindDeployment("ingress-nginx", "ingress-nginx-controller", "180s"); err != nil {
		return err
	}
	if err := ensureKindNamespace("storage"); err != nil {
		return err
	}
	for _, manifest := range []string{"deployment/kind/mysql.yaml", "deployment/kind/minio.yaml"} {
		if err := kindKubectl("apply", "-f", manifest); err != nil {
			return err
		}
	}
	if err := waitForKindDeployment("db", "mysql", "360s"); err != nil {
		return err
	}
	if err := initializeKindDatabase(); err != nil {
		return err
	}
	if err := waitForKindDeployment("storage", "minio", "360s"); err != nil {
		return err
	}
	if err := initializeKindBucket(); err != nil {
		return err
	}

	if err := buildAndLoadKindImages(cluster, true); err != nil {
		return err
	}
	if err := ensureKindNamespace("buildmax"); err != nil {
		return err
	}
	if err := applyKindSecret(); err != nil {
		return err
	}
	if err := applyKindWorkerSeccompProfile(); err != nil {
		return err
	}
	if err := kindKubectl("apply", "-f", "deployment/buildmax-deploy.yaml"); err != nil {
		return err
	}
	// Before any worker Job can be dispatched: a worker pod's securityContext
	// names this DaemonSet's installed file by path, and fails to start on a
	// node it has not yet reached.
	if err := kindKubectl("rollout", "status", "daemonset/buildmax-worker-seccomp", "-n", "buildmax", "--timeout=120s"); err != nil {
		dumpKindNamespace("buildmax")
		return err
	}
	if err := kindKubectl("apply", "-f", "deployment/smoke/mock-llm.kind.yaml"); err != nil {
		return err
	}
	if err := applyKindSmokeConfig(); err != nil {
		return err
	}
	for _, deployment := range []string{"buildmax-smoke-llm", "buildmax-server", "buildmax-portal"} {
		if err := kindKubectl("rollout", "restart", "deployment/"+deployment, "-n", "buildmax"); err != nil {
			return err
		}
	}
	for _, deployment := range []string{"buildmax-smoke-llm", "buildmax-server", "buildmax-portal"} {
		if err := kindKubectl("rollout", "status", "deployment/"+deployment, "-n", "buildmax", "--timeout=180s"); err != nil {
			return err
		}
	}

	if err := kindSmoke(); err != nil {
		fmt.Printf("Kind smoke failed. Run %s kind logs for diagnostics.\n", mk())
		return err
	}
	fmt.Printf("Kind stack is ready at %s (cluster %s).\n", kindPortalURL(), cluster)
	fmt.Printf("Lost the code above? %s kind info issues another one.\n", mk())
	// `exists` the function is shadowed by the cluster check above.
	if _, statErr := os.Stat(localSettingsPath); statErr == nil {
		fmt.Printf("Want the CLI or Desktop to drive this stack with your own models?\n"+
			"  %s kind seed puts the ones in %s into its catalog.\n", mk(), localSettingsPath)
	}
	return nil
}

func kindSmoke() error {
	if err := requireCommands("kubectl"); err != nil {
		return err
	}
	target := kindSmokeTarget()
	if err := runDeploymentSmoke(context.Background(), target); err != nil {
		return err
	}
	printSmokeLogin(target)
	return nil
}

func kindSmokeTarget() smokeTarget {
	return smokeTarget{
		apiBase:              kindPortalURL(),
		portalURL:            kindPortalURL(),
		portalRuntimeAPIBase: "/",
		// Published by deployment/smoke/mock-llm.kind.yaml, and only this one
		// route: a run reaching the model still goes through the in-cluster
		// Service, which is what makes it evidence the deployment wired it up.
		llmControlURL: kindPortalURL() + "/smoke-llm" + llmControlStallPath,
		admin: func(args ...string) (string, error) {
			cmdArgs := append([]string{"--context", kindContext(), "exec", "-n", "buildmax", "deployment/buildmax-server", "--", "buildmax-server"}, args...)
			return captureCombined("kubectl", cmdArgs...)
		},
	}
}

// kindForwardTarget is one in-cluster Service a contributor can reach from this
// machine, with the hints that make the forward useful once it is up.
type kindForwardTarget struct {
	name      string
	namespace string
	service   string
	// hostPort:servicePort, in kubectl's own spelling. The host port matches the
	// service port everywhere here, so a forwarded address reads the same as the
	// in-cluster one.
	ports []string
	hints []string
}

// kindForwardTargets is every Service worth forwarding. The server and Portal
// are absent on purpose: the ingress already publishes them on 8080.
func kindForwardTargets() []kindForwardTarget {
	return []kindForwardTarget{
		{
			name:      "mysql",
			namespace: "db",
			service:   "mysql",
			ports:     []string{"3306:3306"},
			hints: []string{
				"mysql -h 127.0.0.1 -P 3306 -ubuildmax -pbuildmax buildmax",
				"DSN: buildmax:buildmax@tcp(127.0.0.1:3306)/buildmax",
			},
		},
		{
			name:      "minio",
			namespace: "storage",
			service:   "minio",
			ports:     []string{"9000:9000", "9001:9001"},
			hints: []string{
				"Console: http://127.0.0.1:9001 (minio / minio123)",
				"API: http://127.0.0.1:9000, bucket bmstore",
			},
		},
	}
}

// kindForward forwards the cluster's backing services to this machine so a
// client here can read what a run actually wrote.
//
// MySQL and MinIO have ClusterIP Services and the cluster publishes only the
// ingress ports, so there is no way in from the host without this. It runs in
// the foreground and stops with the command: a forward left running in the
// background is a socket into a database that outlives the terminal that
// remembers it exists.
//
// The credentials printed are the development ones in deployment/kind/.
// They are not a secret and not a deployment: those manifests exist to be
// thrown away with the cluster.
func kindForward() error {
	if err := requireCommands("kubectl"); err != nil {
		return err
	}
	cluster := kindClusterName()
	exists, err := kindClusterExists(cluster)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("kind cluster %q does not exist; run %s kind up", cluster, mk())
	}

	var ready []kindForwardTarget
	for _, target := range kindForwardTargets() {
		// Same probe as the cluster preflight, for the same reason: kubectl's own
		// failure names the port but not what is holding it. One busy port skips
		// its target rather than the whole command, so a local MySQL on 3306 does
		// not also cost you MinIO.
		if busy := busyHostPorts(target); len(busy) > 0 {
			fmt.Printf("Skipping %s: host port %s already in use.\n", target.name, strings.Join(busy, " and "))
			fmt.Printf("  Stop what is listening, or forward it yourself: kubectl --context %s -n %s port-forward svc/%s %s\n",
				kindContext(), target.namespace, target.service, strings.Join(placeholderPorts(target), " "))
			continue
		}
		ready = append(ready, target)
	}
	if len(ready) == 0 {
		return errors.New("every forward target's host port is already in use")
	}

	for _, target := range ready {
		fmt.Printf("Forwarding %s.%s.svc.cluster.local from cluster %s\n", target.service, target.namespace, cluster)
		for _, hint := range target.hints {
			fmt.Printf("  %s\n", hint)
		}
	}
	fmt.Println("Leave this running; Ctrl+C stops every forward.")
	return runKindForwards(ready)
}

// placeholderPorts rewrites a target's mappings for the escape-hatch hint, so a
// contributor who has to pick their own host ports is shown every port the
// service needs rather than just the one that collided.
func placeholderPorts(target kindForwardTarget) []string {
	ports := make([]string, 0, len(target.ports))
	for _, mapping := range target.ports {
		ports = append(ports, "<port>:"+strings.SplitN(mapping, ":", 2)[1])
	}
	return ports
}

func busyHostPorts(target kindForwardTarget) []string {
	var busy []string
	for _, mapping := range target.ports {
		hostPort := strings.SplitN(mapping, ":", 2)[0]
		conn, dialErr := net.DialTimeout("tcp", "127.0.0.1:"+hostPort, 300*time.Millisecond)
		if dialErr != nil {
			continue
		}
		_ = conn.Close()
		busy = append(busy, hostPort)
	}
	return busy
}

// runKindForwards runs one kubectl per target until the first one stops.
//
// Whichever ends first ends all of them: a surviving forward would leave the
// contributor holding a stack they believe is fully reachable and is not.
func runKindForwards(targets []kindForwardTarget) error {
	var lines sync.Mutex
	commands := make([]*exec.Cmd, 0, len(targets))
	done := make(chan error, len(targets))
	for _, target := range targets {
		args := append([]string{"--context", kindContext(), "port-forward", "-n", target.namespace, "svc/" + target.service}, target.ports...)
		cmd := exec.Command("kubectl", args...)
		out := &prefixWriter{prefix: target.name, lines: &lines}
		cmd.Stdout, cmd.Stderr = out, out
		if err := cmd.Start(); err != nil {
			stopAll(commands)
			return fmt.Errorf("forward %s: %w", target.name, err)
		}
		commands = append(commands, cmd)
		go func(name string, cmd *exec.Cmd) {
			if err := cmd.Wait(); err != nil && !stoppedOnPurpose(err) {
				done <- fmt.Errorf("forward %s stopped: %w", name, err)
				return
			}
			done <- nil
		}(target.name, cmd)
	}

	// Ctrl+C already reaches every kubectl on its own, because the terminal
	// delivers it to the whole foreground process group. Catching it here keeps
	// this command alive long enough to end as the ordinary stop it is, and
	// covers the case where only this process was signalled.
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	defer signal.Stop(interrupt)
	interrupted := make(chan struct{})
	settled := make(chan struct{})
	defer close(settled)
	go func() {
		select {
		case <-interrupt:
			close(interrupted)
			stopAll(commands)
		case <-settled:
		}
	}()

	var first error
	for i := range commands {
		err := <-done
		if i == 0 {
			first = err
			stopAll(commands)
		}
	}
	select {
	case <-interrupted:
		// The forwards ended because they were asked to, whatever the teardown
		// above made of the children on its way out.
		return nil
	default:
		return first
	}
}

// stoppedOnPurpose reports whether a child ended because something asked it to
// rather than because the forward broke. Ctrl+C is the documented way to stop
// this command, and the terminal delivers it to every kubectl directly, so an
// interrupted child is the ordinary ending rather than a failure to report.
func stoppedOnPurpose(err error) bool {
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		return false
	}
	status, ok := exit.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() {
		return false
	}
	switch status.Signal() {
	case syscall.SIGINT, syscall.SIGTERM:
		return true
	}
	return false
}

func stopAll(commands []*exec.Cmd) {
	for _, cmd := range commands {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
}

// prefixWriter tags each line a child writes with the target it came from, so
// two forwards sharing one terminal stay tellable apart. kubectl reports every
// connection it handles, and "Handling connection for 9000" says nothing on its
// own about which service answered.
type prefixWriter struct {
	prefix  string
	lines   *sync.Mutex
	pending []byte
}

func (w *prefixWriter) Write(p []byte) (int, error) {
	w.lines.Lock()
	defer w.lines.Unlock()
	w.pending = append(w.pending, p...)
	for {
		end := bytes.IndexByte(w.pending, '\n')
		if end < 0 {
			break
		}
		fmt.Printf("%s | %s\n", w.prefix, bytes.TrimRight(w.pending[:end], "\r"))
		w.pending = w.pending[end+1:]
	}
	return len(p), nil
}

// kindInfo prints how to get into the running stack, including a login code.
//
// A login code is single-use and printed once, so a contributor who has lost
// the one `kind up` printed cannot be shown it again — recording it somewhere
// would only save a code that is already spent. Issuing a fresh one is the
// answer, and it is cheap: the account already exists.
func kindInfo(args []string) error {
	if len(args) > 1 {
		return usageErrorf("kind", "info takes at most one email address")
	}
	email := smokeEmail
	if len(args) == 1 && args[0] != "" {
		email = args[0]
	}
	if err := requireCommands("kubectl"); err != nil {
		return err
	}
	cluster := kindClusterName()
	exists, err := kindClusterExists(cluster)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("kind cluster %q does not exist; run %s kind up", cluster, mk())
	}

	fmt.Printf("Cluster: %s (context %s)\n", cluster, kindContext())
	fmt.Printf("Portal:  %s (%s)\n", kindPortalURL(), httpHealth(kindPortalURL()+"/healthz"))
	fmt.Printf("MinIO:   bucket bmstore, key minio, secret minio123\n")
	fmt.Printf("MySQL and MinIO are in-cluster only; reach both with %s kind forward\n", mk())

	code, err := kindSmokeTarget().admin("user", "login-code", email)
	if err != nil {
		return fmt.Errorf("issue a login code for %s: %w", email, err)
	}
	fmt.Printf("\nSign in at %s\n\n%s\n", kindPortalURL(), code)
	fmt.Printf("\nRun %s kind info again for another code.\n", mk())
	return nil
}

// kindStatus reports what the selected cluster is running without changing it,
// so a contributor can tell "nothing deployed" from "deployed but unhealthy"
// before reaching for the much noisier kind logs.
func kindStatus() error {
	if err := requireCommands("kubectl"); err != nil {
		return err
	}
	cluster := kindClusterName()
	fmt.Printf("Cluster: %s (context %s)\n", cluster, kindContext())

	exists, err := kindClusterExists(cluster)
	if err != nil {
		return err
	}
	if !exists {
		fmt.Printf("The cluster does not exist. Run %s kind up.\n", mk())
		return nil
	}
	if err := validateKindPortMapping(cluster); err != nil {
		fmt.Printf("Warning: %v\n", err)
	}
	fmt.Printf("Portal:  %s (%s)\n", kindPortalURL(), httpHealth(kindPortalURL()+"/healthz"))

	printKindSection("Nodes", "get", "nodes", "-o", "wide")
	for _, namespace := range []string{"ingress-nginx", "db", "storage", "buildmax"} {
		printKindSection("Namespace "+namespace, "get", "deployments,jobs,pods", "-n", namespace)
	}
	fmt.Printf("\nRun %s kind logs for events and container logs.\n", mk())
	return nil
}

func printKindSection(title string, args ...string) {
	fmt.Printf("\n%s\n", title)
	if err := kindKubectl(args...); err != nil {
		fmt.Printf("Warning: %v\n", err)
	}
}

func kindLogs() error {
	if err := requireCommands("kubectl"); err != nil {
		return err
	}
	for _, namespace := range []string{"ingress-nginx", "db", "storage", "buildmax"} {
		dumpKindNamespace(namespace)
	}
	return nil
}

func waitForKindDeployment(namespace, name, timeout string) error {
	err := kindKubectl("wait", "--for=condition=Available", "deployment/"+name, "-n", namespace, "--timeout="+timeout)
	if err != nil {
		dumpKindNamespace(namespace)
	}
	return err
}

func dumpKindNamespace(namespace string) {
	commands := [][]string{
		{"get", "pods,jobs,deployments,services,ingresses", "-n", namespace, "-o", "wide"},
		{"get", "events", "-n", namespace, "--sort-by=.lastTimestamp"},
	}

	switch namespace {
	case "ingress-nginx":
		commands = append(commands,
			[]string{"describe", "deployment/ingress-nginx-controller", "-n", namespace},
			[]string{"logs", "-n", namespace, "deployment/ingress-nginx-controller", "--all-containers", "--tail=200"},
		)
	case "db":
		commands = append(commands,
			[]string{"describe", "deployment/mysql", "-n", namespace},
			[]string{"logs", "-n", namespace, "deployment/mysql", "--all-containers", "--tail=200"},
			[]string{"logs", "-n", namespace, "job/mysql-init", "--all-containers", "--tail=200"},
		)
	case "storage":
		commands = append(commands,
			[]string{"describe", "deployment/minio", "-n", namespace},
			[]string{"logs", "-n", namespace, "deployment/minio", "--all-containers", "--tail=200"},
			[]string{"logs", "-n", namespace, "job/minio-init", "--all-containers", "--tail=200"},
		)
	case "buildmax":
		commands = append(commands,
			[]string{"logs", "-n", namespace, "deployment/buildmax-server", "--all-containers", "--tail=200"},
			[]string{"logs", "-n", namespace, "deployment/buildmax-portal", "--all-containers", "--tail=100"},
			[]string{"logs", "-n", namespace, "-l", "job-name", "--all-containers", "--tail=200"},
		)
	}

	for _, args := range commands {
		if err := kindKubectl(args...); err != nil {
			fmt.Printf("Warning: %v\n", err)
		}
	}
}

func kindDown() error {
	// Deleting a cluster still talks to the container engine, so docker is the
	// prerequisite here even though kind itself no longer is.
	if err := requireCommands("docker"); err != nil {
		return err
	}
	cluster := kindClusterName()
	exists, err := kindClusterExists(cluster)
	if err != nil {
		return err
	}
	if !exists {
		fmt.Printf("Kind cluster %q does not exist.\n", cluster)
		return nil
	}
	return runKind("delete", "cluster", "--name", cluster)
}

// checkKindHostPorts fails when something already holds a port the new cluster
// would publish. It checks the two ports this run asked for — the defaults
// unless BUILDMAX_KIND_PORTAL_PORT or BUILDMAX_KIND_TLS_PORT said otherwise —
// rather than reading them back out of the rendered config, so the preflight
// cannot drift from the request it is checking.
func checkKindHostPorts(cluster string) error {
	var busy []string
	for _, port := range []string{kindPortalPort(), kindTLSPort()} {
		// Dial rather than bind. A probe that binds proves nothing here: every
		// listener that matters sets SO_REUSEADDR, and so does net.Listen, so on
		// macOS the probe succeeds against a port that is very much in use. What
		// this needs to know is whether something answers, which is a connection.
		conn, dialErr := net.DialTimeout("tcp", "127.0.0.1:"+port, 300*time.Millisecond)
		if dialErr != nil {
			continue
		}
		_ = conn.Close()
		busy = append(busy, port)
	}
	if len(busy) == 0 {
		return nil
	}
	return fmt.Errorf("host port %s already in use, and kind cluster %q would publish it\n  Stop what is listening, or run another cluster with BUILDMAX_KIND_CLUSTER=<name> BUILDMAX_KIND_PORTAL_PORT=<port> BUILDMAX_KIND_TLS_PORT=<port>",
		strings.Join(busy, " and "), cluster)
}

// renderKindConfig substitutes this run's portal and TLS host ports into the
// committed kind config and writes the result to a temporary file, because
// kind reads its config from a path rather than accepting one inline. The
// substitution is keyed to containerPort (80 and 443), not to the literal
// default host ports, so it survives the committed file's defaults changing.
// The caller must run the returned cleanup once it is done with the file.
func renderKindConfig() (string, func(), error) {
	template, err := os.ReadFile(kindConfigPath)
	if err != nil {
		return "", nil, fmt.Errorf("read %s: %w", kindConfigPath, err)
	}
	replacements := map[string]string{"80": kindPortalPort(), "443": kindTLSPort()}
	lines := strings.Split(string(template), "\n")
	var containerPort string
	for i, line := range lines {
		// The mapping is a YAML sequence, so containerPort carries a leading
		// "- " that hostPort does not.
		trimmed := strings.TrimPrefix(strings.TrimSpace(line), "- ")
		switch {
		case strings.HasPrefix(trimmed, "containerPort:"):
			containerPort = strings.TrimSpace(strings.TrimPrefix(trimmed, "containerPort:"))
		case strings.HasPrefix(trimmed, "hostPort:"):
			if port, ok := replacements[containerPort]; ok {
				indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
				lines[i] = indent + "hostPort: " + port
			}
		}
	}
	file, err := os.CreateTemp("", "buildmax-kind-config-*.yaml")
	if err != nil {
		return "", nil, fmt.Errorf("create a rendered kind config: %w", err)
	}
	if _, err := file.WriteString(strings.Join(lines, "\n")); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return "", nil, fmt.Errorf("write %s: %w", file.Name(), err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(file.Name())
		return "", nil, err
	}
	return file.Name(), func() { _ = os.Remove(file.Name()) }, nil
}

func kindClusterExists(name string) (bool, error) {
	output, err := captureKind("get", "clusters")
	if err != nil {
		return false, fmt.Errorf("list kind clusters: %w", err)
	}
	for _, cluster := range strings.Fields(output) {
		if cluster == name {
			return true, nil
		}
	}
	return false, nil
}

func validateKindPortMapping(cluster string) error {
	output, err := capture("docker", "port", cluster+"-control-plane", "80/tcp")
	if err != nil {
		return fmt.Errorf("inspect ingress port for kind cluster %q: %w", cluster, err)
	}
	want := ":" + kindPortalPort()
	for _, line := range strings.Split(output, "\n") {
		if strings.HasSuffix(strings.TrimSpace(line), want) {
			return nil
		}
	}
	return fmt.Errorf("kind cluster %q does not publish the ingress on port %s; run BUILDMAX_KIND_CLUSTER=%s %s kind down, then kind up with the same BUILDMAX_KIND_PORTAL_PORT", cluster, kindPortalPort(), cluster, mk())
}

func ensureKindNamespace(namespace string) error {
	manifest, err := captureKindKubectl("create", "namespace", namespace, "--dry-run=client", "-o", "yaml")
	if err != nil {
		return fmt.Errorf("render namespace %s: %w", namespace, err)
	}
	return runStdin(manifest, "kubectl", "--context", kindContext(), "apply", "-f", "-")
}

func initializeKindBucket() error {
	_ = kindKubectl("delete", "job/minio-init", "-n", "storage", "--ignore-not-found")
	if err := kindKubectl("apply", "-f", "deployment/kind/minio-init.yaml"); err != nil {
		return err
	}
	if err := kindKubectl("wait", "--for=condition=complete", "job/minio-init", "-n", "storage", "--timeout=180s"); err != nil {
		_ = kindKubectl("logs", "job/minio-init", "-n", "storage")
		return err
	}
	return nil
}

// initializeKindDatabase widens the dev account's grant so the server can
// create whichever schema server.yaml names. See deployment/kind/mysql-init.yaml.
func initializeKindDatabase() error {
	_ = kindKubectl("delete", "job/mysql-init", "-n", "db", "--ignore-not-found")
	if err := kindKubectl("apply", "-f", "deployment/kind/mysql-init.yaml"); err != nil {
		return err
	}
	if err := kindKubectl("wait", "--for=condition=complete", "job/mysql-init", "-n", "db", "--timeout=180s"); err != nil {
		_ = kindKubectl("logs", "job/mysql-init", "-n", "db")
		return err
	}
	return nil
}

func applyKindSecret() error {
	jwt, err := randomHex(32)
	if err != nil {
		return err
	}
	manifest, err := captureKindKubectl(
		"create", "secret", "generic", "buildmax-secret", "-n", "buildmax",
		"--from-literal=BUILDMAX_JWT_SECRET="+jwt,
		"--from-literal=BUILDMAX_DATABASE_PASSWORD=buildmax",
		"--from-literal=BUILDMAX_STORAGE_MINIO_ACCESS_KEY=minio",
		"--from-literal=BUILDMAX_STORAGE_MINIO_SECRET_KEY=minio123",
		"--from-literal=BUILDMAX_CONVERSATION_MODEL_API_KEY=smoke-key",
		"--dry-run=client", "-o", "yaml",
	)
	if err != nil {
		return fmt.Errorf("render local buildmax secret: %w", err)
	}
	return runStdin(manifest, "kubectl", "--context", kindContext(), "apply", "-f", "-")
}

// applyKindWorkerSeccompProfile puts the worker's custom seccomp profile into
// a ConfigMap the DaemonSet in deployment/buildmax-deploy.yaml mounts and
// copies onto every node's kubelet seccomp directory. See
// deployment/seccomp/README.md for what the profile does and why
// RuntimeDefault is not enough.
//
// From a file rather than embedded in buildmax-deploy.yaml: Kubernetes'
// Localhost seccomp type needs the profile on every node's filesystem, not
// inlined in a pod spec, and deployment/seccomp/worker-bwrap.json stays the
// one place this ~600-line profile is edited.
func applyKindWorkerSeccompProfile() error {
	manifest, err := captureKindKubectl(
		"create", "configmap", "buildmax-worker-seccomp", "-n", "buildmax",
		"--from-file=worker-bwrap.json=deployment/seccomp/worker-bwrap.json",
		"--dry-run=client", "-o", "yaml",
	)
	if err != nil {
		return fmt.Errorf("render worker seccomp profile configmap: %w", err)
	}
	return runStdin(manifest, "kubectl", "--context", kindContext(), "apply", "-f", "-")
}

func applyKindSmokeConfig() error {
	return applyKindSmokeConfigFrom("deployment/smoke/server.kind.yaml")
}

// applyKindSmokeConfigFrom mounts path as the server's config, with its
// cors_origin rewritten to this run's portal port. The committed file always
// says 8080: the browser's Origin header is http://localhost:<port>, and the
// server's WebSocket upgrade rejects any other origin, so a cluster on a
// different port needs a config that agrees with it — a mismatch here is
// invisible to every check except an actual conversation turn, which is what
// made it worth getting right rather than leaving the ingress port to imply
// it.
func applyKindSmokeConfigFrom(path string) error {
	renderedPath, cleanup, err := renderKindSmokeConfig(path)
	if err != nil {
		return err
	}
	defer cleanup()
	manifest, err := captureKindKubectl(
		"create", "configmap", "buildmax-config", "-n", "buildmax",
		"--from-file=server.yaml="+renderedPath,
		"--dry-run=client", "-o", "yaml",
	)
	if err != nil {
		return fmt.Errorf("render kind smoke config: %w", err)
	}
	return runStdin(manifest, "kubectl", "--context", kindContext(), "apply", "-f", "-")
}

func renderKindSmokeConfig(path string) (string, func(), error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("read %s: %w", path, err)
	}
	const defaultOrigin = "cors_origin: http://localhost:" + defaultKindPortalPort
	if strings.Count(string(content), defaultOrigin) != 1 {
		return "", nil, fmt.Errorf("%s does not contain exactly one %q line to rewrite", path, defaultOrigin)
	}
	rendered := strings.Replace(string(content), defaultOrigin, "cors_origin: "+kindPortalURL(), 1)
	file, err := os.CreateTemp("", "buildmax-kind-server-*.yaml")
	if err != nil {
		return "", nil, fmt.Errorf("create a rendered server config: %w", err)
	}
	if _, err := file.WriteString(rendered); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return "", nil, fmt.Errorf("write %s: %w", file.Name(), err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(file.Name())
		return "", nil, err
	}
	return file.Name(), func() { _ = os.Remove(file.Name()) }, nil
}

// kindManagedSmoke reruns the smoke against a cluster switched to managed
// task-run inference.
//
// It swaps the ConfigMap and restarts the server rather than standing up a
// second cluster: transport is startup configuration, so the server has to
// reread it, but everything else about the deployment is the same. Worker pods
// pick up the new file because each Job mounts the ConfigMap when it starts.
//
// The cluster is left in managed mode afterwards. Rerun `./make kind up` to put
// it back, which is also what a contributor would do to get a clean stack.
func kindManagedSmoke() error {
	if err := requireCommands("kubectl"); err != nil {
		return err
	}
	if err := applyKindSmokeConfigFrom("deployment/smoke/server.kind.managed.yaml"); err != nil {
		return err
	}
	if err := kindKubectl("rollout", "restart", "deployment/buildmax-server", "-n", "buildmax"); err != nil {
		return err
	}
	if err := kindKubectl("rollout", "status", "deployment/buildmax-server", "-n", "buildmax", "--timeout=180s"); err != nil {
		return err
	}
	target := kindSmokeTarget()
	target.managedLLM = true
	if err := runDeploymentSmoke(context.Background(), target); err != nil {
		return err
	}
	printSmokeLogin(target)
	return nil
}

func randomHex(bytes int) (string, error) {
	data := make([]byte, bytes)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("generate local secret: %w", err)
	}
	return hex.EncodeToString(data), nil
}

func requireCommands(names ...string) error {
	var missing []string
	for _, name := range names {
		if !have(name) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required commands: %s", strings.Join(missing, ", "))
	}
	return nil
}
