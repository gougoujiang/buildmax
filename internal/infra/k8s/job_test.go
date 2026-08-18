package k8s

import (
	"context"
	"regexp"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/model"
)

// fakeJobCreator records the last created Job for tests.
type fakeJobCreator struct {
	lastJob *batchv1.Job
}

func (f *fakeJobCreator) CreateJob(ctx context.Context, namespace string, job *batchv1.Job) error {
	f.lastJob = job.DeepCopy()
	return nil
}

func TestK8sJobRunner_Run_SetsJobNamePattern(t *testing.T) {
	fake := &fakeJobCreator{}
	runner := NewK8sJobRunner("buildmax", "buildmax:local", []corev1.EnvVar{}, PodConfig{}, fake)
	run := model.TaskRun{TaskRunID: "01ARZ3NDEKTSV4RRFFQ69G5FAV", TaskID: "chat1", Status: "SCHEDULED"}

	workerType, k8sName, k8sAt, err := runner.Run(context.Background(), run, "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if workerType != "k8s_job" {
		t.Errorf("workerType = %q, want k8s_job", workerType)
	}
	if k8sName == nil || *k8sName == "" {
		t.Error("k8sJobName should be set")
	}
	if k8sAt == nil || *k8sAt <= 0 {
		t.Error("k8sJobCreatedAt should be set")
	}
	pattern := regexp.MustCompile(`^buildmax-worker-[a-z0-9-]+-\d+$`)
	if !pattern.MatchString(*k8sName) {
		t.Errorf("job name %q does not match pattern buildmax-worker-<id>-<timestamp>", *k8sName)
	}
	if fake.lastJob == nil || fake.lastJob.Name != *k8sName {
		t.Errorf("fake Job name = %v, want %q", fake.lastJob.Name, *k8sName)
	}
}

// TestK8sJobRunner_MountsServerConfig covers the contract the k8s deployment
// path depends on: a worker pod gets server.yaml mounted from the ConfigMap and
// BUILDMAX_HOME pointing at the directory it lands in. Without both, the worker
// falls back to built-in defaults and cannot reach the database or storage.
func TestK8sJobRunner_MountsServerConfig(t *testing.T) {
	fake := &fakeJobCreator{}
	inherited := []corev1.EnvVar{
		{Name: "BUILDMAX_WORKER_TOKEN", Value: "secret"},
		{Name: config.EnvKeyBuildmaxHome, Value: "/server-side-path"},
	}
	runner := NewK8sJobRunner("buildmax", "buildmax:local", inherited,
		PodConfig{ConfigMapName: "buildmax-config", HomeDir: "/buildmax"}, fake)

	if _, _, _, err := runner.Run(context.Background(), model.TaskRun{TaskRunID: "r_1"}, ""); err != nil {
		t.Fatalf("Run: %v", err)
	}
	container := fake.lastJob.Spec.Template.Spec.Containers[0]

	// BUILDMAX_HOME must be the pod's path exactly once — the server's own value
	// names a directory that does not exist in the worker pod.
	var homeValues []string
	for _, e := range container.Env {
		if e.Name == config.EnvKeyBuildmaxHome {
			homeValues = append(homeValues, e.Value)
		}
	}
	if len(homeValues) != 1 || homeValues[0] != "/buildmax" {
		t.Errorf("%s in pod = %v, want exactly [/buildmax]", config.EnvKeyBuildmaxHome, homeValues)
	}

	// Credentials inherited from the server must survive.
	var sawToken bool
	for _, e := range container.Env {
		if e.Name == "BUILDMAX_WORKER_TOKEN" && e.Value == "secret" {
			sawToken = true
		}
	}
	if !sawToken {
		t.Error("inherited BUILDMAX_WORKER_TOKEN was not propagated to the worker pod")
	}

	// server.yaml must be mounted by subPath so the rest of BUILDMAX_HOME stays
	// writable for sessions, logs, and traces.
	var configMount *corev1.VolumeMount
	for i, m := range container.VolumeMounts {
		if m.MountPath == "/buildmax/server.yaml" {
			configMount = &container.VolumeMounts[i]
		}
	}
	if configMount == nil {
		t.Fatalf("server.yaml is not mounted; mounts = %+v", container.VolumeMounts)
	}
	if configMount.SubPath != "server.yaml" {
		t.Errorf("config mount SubPath = %q, want server.yaml", configMount.SubPath)
	}

	var sawConfigMap, sawHomeVolume bool
	for _, v := range fake.lastJob.Spec.Template.Spec.Volumes {
		if v.ConfigMap != nil && v.ConfigMap.Name == "buildmax-config" {
			sawConfigMap = true
		}
		if v.EmptyDir != nil {
			sawHomeVolume = true
		}
	}
	if !sawConfigMap {
		t.Error("worker pod has no buildmax-config ConfigMap volume")
	}
	if !sawHomeVolume {
		t.Error("worker pod has no writable volume for BUILDMAX_HOME")
	}
}

// TestK8sJobRunner_NoConfigMap keeps the degenerate case explicit: with no
// ConfigMap configured the pod still gets a writable home, just no config file.
func TestK8sJobRunner_NoConfigMap(t *testing.T) {
	fake := &fakeJobCreator{}
	runner := NewK8sJobRunner("buildmax", "buildmax:local", nil, PodConfig{}, fake)
	if _, _, _, err := runner.Run(context.Background(), model.TaskRun{TaskRunID: "r_2"}, ""); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, v := range fake.lastJob.Spec.Template.Spec.Volumes {
		if v.ConfigMap != nil {
			t.Errorf("unexpected ConfigMap volume %q when none is configured", v.ConfigMap.Name)
		}
	}
	container := fake.lastJob.Spec.Template.Spec.Containers[0]
	paths := make(map[string]bool, len(container.VolumeMounts))
	for _, m := range container.VolumeMounts {
		paths[m.MountPath] = true
	}
	// The default home plus the writable /tmp a read-only root filesystem
	// requires. Nothing config-shaped.
	if !paths["/buildmax"] || !paths["/tmp"] || len(paths) != 2 {
		t.Errorf("mounts = %+v, want the default home at /buildmax and a writable /tmp", container.VolumeMounts)
	}
}

// TestWorkerEnvFromEnviron_WithholdsServerOnlyCredentials asserts the Job pod
// gets what a worker reads and nothing else. The server process holds the JWT
// signing secret and the database password; forwarding them would give every
// model-chosen command in the pod the ability to mint tokens for any user and
// read the whole database.
func TestWorkerEnvFromEnviron_WithholdsServerOnlyCredentials(t *testing.T) {
	t.Setenv(config.EnvKeyBuildmaxWorkerToken, "wt-secret")
	t.Setenv(config.EnvKeyBuildmaxMinIOAccessKey, "minio-key")
	t.Setenv(config.EnvKeyBuildmaxJWTSecret, "jwt-secret")
	t.Setenv(config.EnvKeyBuildmaxDatabasePassword, "db-secret")
	t.Setenv("BUILDMAX_ADDED_LATER", "unknown")
	t.Setenv("PATH_LIKE_NON_BUILDMAX", "ignored")

	got := WorkerEnvFromEnviron(false)
	byName := make(map[string]string, len(got))
	for _, e := range got {
		byName[e.Name] = e.Value
	}

	for _, name := range []string{config.EnvKeyBuildmaxJWTSecret, config.EnvKeyBuildmaxDatabasePassword, "BUILDMAX_ADDED_LATER"} {
		if _, ok := byName[name]; ok {
			t.Errorf("%s must not be forwarded to a worker pod", name)
		}
	}
	for _, name := range []string{config.EnvKeyBuildmaxWorkerToken, config.EnvKeyBuildmaxMinIOAccessKey} {
		if _, ok := byName[name]; !ok {
			t.Errorf("%s is read by a worker but was not forwarded", name)
		}
	}
	if _, ok := byName["PATH_LIKE_NON_BUILDMAX"]; ok {
		t.Error("only BUILDMAX_ variables belong in the pod env built here")
	}
}

// TestJobPodIsConfined asserts the containment a worker pod is created with.
// A worker runs model-chosen shell commands, so each of these is load-bearing
// rather than hygiene, and each has silently regressed elsewhere before.
func TestJobPodIsConfined(t *testing.T) {
	fake := &fakeJobCreator{}
	r := NewK8sJobRunner("buildmax", "buildmax:local", nil, PodConfig{ConfigMapName: "buildmax-config"}, fake)
	if _, _, _, err := r.Run(context.Background(), model.TaskRun{TaskRunID: "run-1"}, ""); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if fake.lastJob == nil {
		t.Fatal("no job created")
	}
	spec := fake.lastJob.Spec.Template.Spec

	if spec.AutomountServiceAccountToken == nil || *spec.AutomountServiceAccountToken {
		t.Error("a worker never calls the Kubernetes API; its token must not be mounted")
	}
	psc := spec.SecurityContext
	if psc == nil {
		t.Fatal("pod security context missing")
	}
	if psc.RunAsNonRoot == nil || !*psc.RunAsNonRoot {
		t.Error("worker pods must not run as root")
	}
	if psc.RunAsUser == nil || *psc.RunAsUser != defaultWorkerUID {
		t.Errorf("run as uid = %v, want %d", psc.RunAsUser, defaultWorkerUID)
	}
	if psc.FSGroup == nil || *psc.FSGroup != defaultWorkerUID {
		t.Error("fsGroup must match the uid or the mounted volumes are unwritable")
	}

	csc := spec.Containers[0].SecurityContext
	if csc == nil {
		t.Fatal("container security context missing")
	}
	if csc.AllowPrivilegeEscalation == nil || *csc.AllowPrivilegeEscalation {
		t.Error("privilege escalation must be denied")
	}
	if csc.ReadOnlyRootFilesystem == nil || !*csc.ReadOnlyRootFilesystem {
		t.Error("the root filesystem must be read-only")
	}
	if len(csc.Capabilities.Drop) != 1 || csc.Capabilities.Drop[0] != "ALL" {
		t.Errorf("capabilities drop = %v, want [ALL]", csc.Capabilities.Drop)
	}

	// A read-only root filesystem without a writable /tmp breaks ordinary
	// tooling in ways that look like the tool is broken, so the two ship
	// together or not at all.
	var hasTmp bool
	for _, m := range spec.Containers[0].VolumeMounts {
		if m.MountPath == "/tmp" {
			hasTmp = true
		}
	}
	if !hasTmp {
		t.Error("a read-only root filesystem needs a writable /tmp")
	}
}

// TestJobPodResources covers both directions: configured limits reach the pod,
// and an unconfigured deployment stays unbounded rather than inheriting a
// number nobody chose.
func TestJobPodResources(t *testing.T) {
	t.Run("configured", func(t *testing.T) {
		fake := &fakeJobCreator{}
		r := NewK8sJobRunner("buildmax", "img", nil, PodConfig{
			Resources: PodResources{CPURequest: "250m", CPULimit: "2", MemoryRequest: "512Mi", MemoryLimit: "4Gi"},
		}, fake)
		if _, _, _, err := r.Run(context.Background(), model.TaskRun{TaskRunID: "run-1"}, ""); err != nil {
			t.Fatalf("Run: %v", err)
		}
		res := fake.lastJob.Spec.Template.Spec.Containers[0].Resources
		if got := res.Limits.Memory().String(); got != "4Gi" {
			t.Errorf("memory limit = %s, want 4Gi", got)
		}
		if got := res.Requests.Cpu().String(); got != "250m" {
			t.Errorf("cpu request = %s, want 250m", got)
		}
	})

	t.Run("unset stays unbounded", func(t *testing.T) {
		fake := &fakeJobCreator{}
		r := NewK8sJobRunner("buildmax", "img", nil, PodConfig{}, fake)
		if _, _, _, err := r.Run(context.Background(), model.TaskRun{TaskRunID: "run-1"}, ""); err != nil {
			t.Fatalf("Run: %v", err)
		}
		res := fake.lastJob.Spec.Template.Spec.Containers[0].Resources
		if len(res.Limits) != 0 || len(res.Requests) != 0 {
			t.Errorf("an unconfigured deployment must stay unbounded, got %+v", res)
		}
	})

	t.Run("unparseable value is dropped, not fatal", func(t *testing.T) {
		fake := &fakeJobCreator{}
		r := NewK8sJobRunner("buildmax", "img", nil, PodConfig{
			Resources: PodResources{MemoryLimit: "4 gigabytes", CPULimit: "1"},
		}, fake)
		if _, _, _, err := r.Run(context.Background(), model.TaskRun{TaskRunID: "run-1"}, ""); err != nil {
			t.Fatalf("a typo in a limit must not stop a run: %v", err)
		}
		res := fake.lastJob.Spec.Template.Spec.Containers[0].Resources
		if _, ok := res.Limits["memory"]; ok {
			t.Error("the unparseable memory limit should have been dropped")
		}
		if got := res.Limits.Cpu().String(); got != "1" {
			t.Errorf("the valid cpu limit should survive, got %s", got)
		}
	})
}

// TestK8sJobRunner_CarriesRunToken covers how a run's gateway credential reaches
// the pod. It is per run, so it cannot be part of the inherited environment the
// runner was built with, and a deployment that mints none must not gain an empty
// variable that looks like a configured one.
func TestK8sJobRunner_CarriesRunToken(t *testing.T) {
	envValues := func(job *batchv1.Job, name string) []string {
		var out []string
		for _, e := range job.Spec.Template.Spec.Containers[0].Env {
			if e.Name == name {
				out = append(out, e.Value)
			}
		}
		return out
	}

	t.Run("minted", func(t *testing.T) {
		fake := &fakeJobCreator{}
		runner := NewK8sJobRunner("buildmax", "buildmax:local", nil, PodConfig{HomeDir: "/buildmax"}, fake)
		if _, _, _, err := runner.Run(context.Background(), model.TaskRun{TaskRunID: "r_1"}, "signed-token"); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if got := envValues(fake.lastJob, config.EnvKeyBuildmaxRunToken); len(got) != 1 || got[0] != "signed-token" {
			t.Errorf("%s in pod = %v, want exactly [signed-token]", config.EnvKeyBuildmaxRunToken, got)
		}
	})

	t.Run("none", func(t *testing.T) {
		fake := &fakeJobCreator{}
		runner := NewK8sJobRunner("buildmax", "buildmax:local", nil, PodConfig{HomeDir: "/buildmax"}, fake)
		if _, _, _, err := runner.Run(context.Background(), model.TaskRun{TaskRunID: "r_2"}, ""); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if got := envValues(fake.lastJob, config.EnvKeyBuildmaxRunToken); len(got) != 0 {
			t.Errorf("%s in pod = %v, want it absent", config.EnvKeyBuildmaxRunToken, got)
		}
	})
}

// TestWorkerEnvFromEnviron_DropsInheritedRunToken records that the only run
// token a worker can find is the one minted for its own run. A stale value in
// the server's environment must not travel into a pod, where it would name some
// other run.
func TestWorkerEnvFromEnviron_DropsInheritedRunToken(t *testing.T) {
	t.Setenv(config.EnvKeyBuildmaxRunToken, "some-other-runs-token")
	t.Setenv("BUILDMAX_WORKER_TOKEN", "kept")

	env := WorkerEnvFromEnviron(false)
	var sawWorkerToken bool
	for _, e := range env {
		if e.Name == config.EnvKeyBuildmaxRunToken {
			t.Errorf("%s was inherited from the server environment", config.EnvKeyBuildmaxRunToken)
		}
		if e.Name == "BUILDMAX_WORKER_TOKEN" {
			sawWorkerToken = true
		}
	}
	if !sawWorkerToken {
		t.Error("BUILDMAX_WORKER_TOKEN was not propagated; the filter dropped too much")
	}
}

// TestWorkerEnvFromEnviron_ManagedPodHasNoProviderKey covers the pod half of
// removing the upstream credential. A managed run reaches models through the
// server, so a provider key in the pod would be a secret with no purpose sitting
// where model-chosen commands run.
func TestWorkerEnvFromEnviron_ManagedPodHasNoProviderKey(t *testing.T) {
	t.Setenv(config.EnvKeyBuildmaxConversationAPIKey, "provider-key")
	t.Setenv(config.EnvKeyBuildmaxWorkerToken, "wt-secret")

	names := func(env []corev1.EnvVar) map[string]string {
		out := make(map[string]string, len(env))
		for _, e := range env {
			out[e.Name] = e.Value
		}
		return out
	}

	managed := names(WorkerEnvFromEnviron(true))
	if _, ok := managed[config.EnvKeyBuildmaxConversationAPIKey]; ok {
		t.Errorf("%s reached a managed worker pod", config.EnvKeyBuildmaxConversationAPIKey)
	}
	if _, ok := managed[config.EnvKeyBuildmaxWorkerToken]; !ok {
		t.Error("managed filtering dropped the worker token, which every run still needs")
	}

	if _, ok := names(WorkerEnvFromEnviron(false))[config.EnvKeyBuildmaxConversationAPIKey]; !ok {
		t.Errorf("%s was withheld from a direct worker pod", config.EnvKeyBuildmaxConversationAPIKey)
	}
}
