package k8s

import (
	"context"
	"log/slog"
	"os"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
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
}

func (p PodConfig) homeDir() string {
	if p.HomeDir == "" {
		return "/buildmax"
	}
	return p.HomeDir
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
// home directory. Any inherited BUILDMAX_HOME is dropped: it names a path on the
// server, which does not exist in the worker pod.
func (r *K8sJobRunner) podEnv() []corev1.EnvVar {
	out := make([]corev1.EnvVar, 0, len(r.env)+1)
	for _, e := range r.env {
		if e.Name == config.EnvKeyBuildmaxHome {
			continue
		}
		out = append(out, e)
	}
	return append(out, corev1.EnvVar{Name: config.EnvKeyBuildmaxHome, Value: r.pod.homeDir()})
}

// podVolumes returns the writable home volume plus, when configured, the
// ConfigMap carrying server.yaml.
func (r *K8sJobRunner) podVolumes() ([]corev1.Volume, []corev1.VolumeMount) {
	home := r.pod.homeDir()
	volumes := []corev1.Volume{{
		Name:         homeVolumeName,
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
	}}
	mounts := []corev1.VolumeMount{{Name: homeVolumeName, MountPath: home}}

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

// Run creates a Job for the task run; on success returns ("k8s_job", &jobName, &createdAtUnix, nil). On failure returns error.
func (r *K8sJobRunner) Run(ctx context.Context, run model.TaskRun) (workerType string, k8sJobName *string, k8sJobCreatedAt *int64, err error) {
	jobName := util.WorkerJobNameForTaskRun(run.TaskRunID)
	now := metav1.Now()
	createdAtUnix := now.Unix()

	volumes, mounts := r.podVolumes()
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: r.namespace},
		Spec: batchv1.JobSpec{
			BackoffLimit: util.Ptr(int32(3)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Volumes:       volumes,
					Containers: []corev1.Container{
						{
							Name:         "worker",
							Image:        r.image,
							Command:      []string{"buildmax-worker"},
							Args:         []string{"--task-run-id", run.TaskRunID},
							Env:          r.podEnv(),
							VolumeMounts: mounts,
						},
					},
				},
			},
		},
	}

	if err := r.client.CreateJob(ctx, r.namespace, job); err != nil {
		slog.Warn("scheduler: failed to create k8s Job", "task_run_id", run.TaskRunID, "job_name", jobName, "err", err)
		return "", nil, nil, err
	}
	slog.Info("scheduler: created k8s Job", "task_run_id", run.TaskRunID, "job_name", jobName, "namespace", r.namespace)
	return "k8s_job", &jobName, &createdAtUnix, nil
}

// jobCreatorImpl implements JobCreator using a Kubernetes clientset.
type jobCreatorImpl struct {
	clientset *kubernetes.Clientset
}

func (c *jobCreatorImpl) CreateJob(ctx context.Context, namespace string, job *batchv1.Job) error {
	_, err := c.clientset.BatchV1().Jobs(namespace).Create(ctx, job, metav1.CreateOptions{})
	return err
}

// WorkerEnvFromEnviron returns BUILDMAX_* environment variables from the current process as corev1.EnvVar for Job pods.
func WorkerEnvFromEnviron() []corev1.EnvVar {
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
