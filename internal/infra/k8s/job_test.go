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

	workerType, k8sName, k8sAt, err := runner.Run(context.Background(), run)
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

	if _, _, _, err := runner.Run(context.Background(), model.TaskRun{TaskRunID: "r_1"}); err != nil {
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
	if _, _, _, err := runner.Run(context.Background(), model.TaskRun{TaskRunID: "r_2"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, v := range fake.lastJob.Spec.Template.Spec.Volumes {
		if v.ConfigMap != nil {
			t.Errorf("unexpected ConfigMap volume %q when none is configured", v.ConfigMap.Name)
		}
	}
	container := fake.lastJob.Spec.Template.Spec.Containers[0]
	if len(container.VolumeMounts) != 1 || container.VolumeMounts[0].MountPath != "/buildmax" {
		t.Errorf("mounts = %+v, want only the default home at /buildmax", container.VolumeMounts)
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

	got := WorkerEnvFromEnviron()
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
