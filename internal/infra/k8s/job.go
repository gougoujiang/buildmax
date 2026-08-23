package k8s

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/util"
)

// JobCreator creates Kubernetes Jobs. Used by K8sJobRunner; can be implemented by a clientset wrapper or a fake in tests.
type JobCreator interface {
	CreateJob(ctx context.Context, namespace string, job *batchv1.Job) error
}

// Volume and mount names for the worker pod's configuration.
const (
	homeVolumeName   = "buildmax-home"
	configVolumeName = "buildmax-config"
	tmpVolumeName    = "tmp"
	serverConfigFile = "server.yaml"
)

// PodConfig describes how a worker pod obtains its configuration.
//
// A worker reads server.yaml from BUILDMAX_HOME exactly as the server does, so
// the pod needs that file mounted and BUILDMAX_HOME pointing at it. Inherited
// environment variables cover credentials only; they are not a substitute for
// the file.
type PodConfig struct {
	// ConfigMapName holds a server.yaml key, mounted read-only into the pod.
	// Empty mounts no file — the worker then has defaults plus inherited env.
	ConfigMapName string
	// HomeDir becomes BUILDMAX_HOME in the pod and is where server.yaml lands.
	// Empty defaults to /buildmax.
	HomeDir string
	// RunAsUser is the uid the worker runs as. Empty defaults to
	// defaultWorkerUID. Clusters that assign their own uid ranges — OpenShift
	// most commonly — set this to a value inside their range.
	RunAsUser int64
	// Resources bounds the pod's CPU and memory. Zero values leave the
	// corresponding request or limit unset, which is what an unconfigured
	// deployment gets: no limit, exactly as before this field existed. The
	// reference manifest sets them.
	Resources PodResources
}

// PodResources holds Kubernetes quantity strings, e.g. "500m" or "1Gi".
type PodResources struct {
	CPURequest    string
	CPULimit      string
	MemoryRequest string
	MemoryLimit   string
}

// defaultWorkerUID is an unprivileged uid with no meaning to the image. The
// worker writes only into mounted volumes, whose ownership fsGroup fixes up, so
// nothing in the image needs to belong to this user.
const defaultWorkerUID int64 = 65532

func (p PodConfig) homeDir() string {
	if p.HomeDir == "" {
		return "/buildmax"
	}
	return p.HomeDir
}

func (p PodConfig) runAsUser() int64 {
	if p.RunAsUser == 0 {
		return defaultWorkerUID
	}
	return p.RunAsUser
}

// resourceRequirements converts the configured quantities. An unparseable value
// is dropped with a warning rather than failing the run: a typo in a limit
// should not stop a deployment from executing work.
func (p PodConfig) resourceRequirements() corev1.ResourceRequirements {
	out := corev1.ResourceRequirements{}
	add := func(list *corev1.ResourceList, name corev1.ResourceName, value, field string) {
		if value == "" {
			return
		}
		q, err := resource.ParseQuantity(value)
		if err != nil {
			slog.Warn("k8s: ignoring unparseable worker resource value", "field", field, "value", value, "err", err)
			return
		}
		if *list == nil {
			*list = corev1.ResourceList{}
		}
		(*list)[name] = q
	}
	add(&out.Requests, corev1.ResourceCPU, p.Resources.CPURequest, "cpu_request")
	add(&out.Requests, corev1.ResourceMemory, p.Resources.MemoryRequest, "memory_request")
	add(&out.Limits, corev1.ResourceCPU, p.Resources.CPULimit, "cpu_limit")
	add(&out.Limits, corev1.ResourceMemory, p.Resources.MemoryLimit, "memory_limit")
	return out
}

// podSecurityContext confines the worker pod.
//
// A worker executes model-chosen shell commands, so the pod is treated as
// running untrusted code even though the team that submitted the task is
// trusted: the prompt, the repository content, and the tool output that steer
// those commands are not.
//
// runAsNonRoot with an explicit uid needs no cooperation from the image, and
// fsGroup makes the mounted volumes writable by it.
func (p PodConfig) podSecurityContext() *corev1.PodSecurityContext {
	uid := p.runAsUser()
	return &corev1.PodSecurityContext{
		RunAsNonRoot:   util.Ptr(true),
		RunAsUser:      util.Ptr(uid),
		RunAsGroup:     util.Ptr(uid),
		FSGroup:        util.Ptr(uid),
		SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
	}
}

// containerSecurityContext drops everything the worker does not need. The
// binary writes only into mounted volumes, so the root filesystem is read-only;
// tmpVolumeName restores the one writable path shell commands assume.
func containerSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		AllowPrivilegeEscalation: util.Ptr(false),
		ReadOnlyRootFilesystem:   util.Ptr(true),
		Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
	}
}

// K8sJobRunner creates a Kubernetes Job per task. Run returns immediately after Job create.
type K8sJobRunner struct {
	namespace string
	image     string
	env       []corev1.EnvVar
	pod       PodConfig
	client    JobCreator
}

// NewK8sJobRunner returns a runner that creates a Job in the given namespace with
// the given image, inherited env, and configuration source for the worker pod.
func NewK8sJobRunner(namespace, image string, env []corev1.EnvVar, pod PodConfig, client JobCreator) *K8sJobRunner {
	return &K8sJobRunner{namespace: namespace, image: image, env: env, pod: pod, client: client}
}

// podEnv returns the inherited environment with BUILDMAX_HOME set to the pod's
// home directory, plus this run's gateway credential when one was minted. Any
// inherited BUILDMAX_HOME is dropped: it names a path on the server, which does
// not exist in the worker pod.
//
// The run token is a plain value in the Job spec, so anyone who can read Job
// objects in this namespace can read it. That is a known limit of the first
// version — the token names one run, expires, and stops working when the run
// does. See docs/design/worker-run-token.md.
func (r *K8sJobRunner) podEnv(runToken string) []corev1.EnvVar {
	out := make([]corev1.EnvVar, 0, len(r.env)+2)
	for _, e := range r.env {
		if e.Name == config.EnvKeyBuildmaxHome {
			continue
		}
		out = append(out, e)
	}
	out = append(out, corev1.EnvVar{Name: config.EnvKeyBuildmaxHome, Value: r.pod.homeDir()})
	if runToken != "" {
		out = append(out, corev1.EnvVar{Name: config.EnvKeyBuildmaxRunToken, Value: runToken})
	}
	return out
}

// podVolumes returns the writable home volume plus, when configured, the
// ConfigMap carrying server.yaml.
func (r *K8sJobRunner) podVolumes() ([]corev1.Volume, []corev1.VolumeMount) {
	home := r.pod.homeDir()
	volumes := []corev1.Volume{{
		Name:         homeVolumeName,
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
	}, {
		// The root filesystem is read-only, and shell commands assume a
		// writable /tmp. Without this the worker runs but ordinary tooling
		// fails in ways that look like the tool is broken.
		Name:         tmpVolumeName,
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
	}}
	mounts := []corev1.VolumeMount{
		{Name: homeVolumeName, MountPath: home},
		{Name: tmpVolumeName, MountPath: "/tmp"},
	}

	if r.pod.ConfigMapName == "" {
		return volumes, mounts
	}
	volumes = append(volumes, corev1.Volume{
		Name: configVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: r.pod.ConfigMapName},
			},
		},
	})
	// subPath mounts the single file, leaving the rest of BUILDMAX_HOME writable
	// for sessions, logs, and traces.
	mounts = append(mounts, corev1.VolumeMount{
		Name:      configVolumeName,
		MountPath: home + "/" + serverConfigFile,
		SubPath:   serverConfigFile,
		ReadOnly:  true,
	})
	return volumes, mounts
}

// Run creates a Job for the task run; on success returns ("k8s_job", &jobName, &createdAt, nil). On failure returns error.
//
// runToken is this run's managed-gateway credential, or "" when the deployment
// mints none.
func (r *K8sJobRunner) Run(ctx context.Context, run model.TaskRun, runToken string) (workerType string, k8sJobName *string, k8sJobCreatedAt *time.Time, err error) {
	jobName := util.WorkerJobNameForTaskRun(run.ID)
	now := metav1.Now()
	createdAt := now.UTC()

	volumes, mounts := r.podVolumes()
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: r.namespace},
		Spec: batchv1.JobSpec{
			BackoffLimit: util.Ptr(int32(3)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					// A drained or evicted worker stops its agent, uploads what
					// the run produced, and reports the outcome. That window is
					// bounded by taskrun.interruptReportTimeout; this is the
					// deadline it has to fit inside, set explicitly rather than
					// inherited so the two are visible together. See
					// docs/design/graceful-shutdown.md §6.3.
					TerminationGracePeriodSeconds: util.Ptr(int64(30)),
					Volumes:                       volumes,
					// A worker never calls the Kubernetes API, so a mounted
					// service-account token is only a credential for
					// model-chosen commands to find.
					AutomountServiceAccountToken: util.Ptr(false),
					SecurityContext:              r.pod.podSecurityContext(),
					Containers: []corev1.Container{
						{
							Name:            "worker",
							Image:           r.image,
							Command:         []string{"buildmax-worker"},
							Args:            []string{"--task-run-id", run.ID},
							Env:             r.podEnv(runToken),
							VolumeMounts:    mounts,
							SecurityContext: containerSecurityContext(),
							Resources:       r.pod.resourceRequirements(),
						},
					},
				},
			},
		},
	}

	if err := r.client.CreateJob(ctx, r.namespace, job); err != nil {
		componentLog().Warn("failed to create k8s Job", "task_run_id", run.ID, "job_name", jobName, "err", err)
		return "", nil, nil, err
	}
	componentLog().Info("created k8s Job", "task_run_id", run.ID, "job_name", jobName, "namespace", r.namespace)
	return "k8s_job", &jobName, &createdAt, nil
}

// jobCreatorImpl implements JobCreator using a Kubernetes clientset.
type jobCreatorImpl struct {
	clientset *kubernetes.Clientset
}

func (c *jobCreatorImpl) CreateJob(ctx context.Context, namespace string, job *batchv1.Job) error {
	_, err := c.clientset.BatchV1().Jobs(namespace).Create(ctx, job, metav1.CreateOptions{})
	return err
}

// WorkerEnvFromEnviron returns the environment a worker Job pod is given: the
// BUILDMAX_* variables from this process that a worker actually reads.
//
// The server holds credentials a worker has no use for — the JWT signing
// secret, the database password — and forwarding them would hand every
// model-chosen command the ability to mint tokens for any user and read the
// whole database. config.WorkerNeedsEnv decides; see its comment for how to
// extend the set.
//
// managedLLM says whether task runs reach models through the server, which
// withholds the provider credential as well.
func WorkerEnvFromEnviron(managedLLM bool) []corev1.EnvVar {
	env := os.Environ()
	var out []corev1.EnvVar
	for _, e := range env {
		if !strings.HasPrefix(e, "BUILDMAX_") {
			continue
		}
		idx := strings.IndexByte(e, '=')
		if idx <= 0 {
			continue
		}
		if !config.WorkerNeedsEnv(e[:idx], managedLLM) {
			continue
		}
		out = append(out, corev1.EnvVar{Name: e[:idx], Value: e[idx+1:]})
	}
	return out
}

// BuildK8sJobCreator builds rest config (in-cluster or kubeconfig), creates a clientset, and returns a JobCreator.
// For use when worker.run_mode is k8s_job in server.yaml. Returns an error if not in cluster and no usable kubeconfig.
func BuildK8sJobCreator() (JobCreator, error) {
	restCfg, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := os.Getenv("KUBECONFIG")
		restCfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, err
		}
	}
	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, err
	}
	return &jobCreatorImpl{clientset: clientset}, nil
}

// Identity belongs in an attr, not in every message string.
func componentLog() *slog.Logger { return slog.With("component", "k8s_runner") }
