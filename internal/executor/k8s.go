package executor

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

	"buildmax/internal/storage/entity"
)

// JobCreator creates Kubernetes Jobs. Used by K8sJobRunner; can be implemented by a clientset wrapper or a fake in tests.
type JobCreator interface {
	CreateJob(ctx context.Context, namespace string, job *batchv1.Job) error
}

// K8sJobRunner creates a Kubernetes Job per task. Run returns immediately after Job create.
type K8sJobRunner struct {
	namespace string
	image     string
	env       []corev1.EnvVar
	client    JobCreator
}

// NewK8sJobRunner returns a runner that creates a Job in the given namespace with the given image and env.
func NewK8sJobRunner(namespace, image string, env []corev1.EnvVar, client JobCreator) *K8sJobRunner {
	return &K8sJobRunner{namespace: namespace, image: image, env: env, client: client}
}

// Run creates a Job for the task; on success returns ("k8s_job", &jobName, &createdAtUnix, nil). On failure returns error.
func (r *K8sJobRunner) Run(ctx context.Context, task entity.Task) (workerType string, k8sJobName *string, k8sJobCreatedAt *int64, err error) {
	jobName := jobNameForTask(task.TaskID)
	now := metav1.Now()
	createdAtUnix := now.Unix()

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: r.namespace},
		Spec: batchv1.JobSpec{
			BackoffLimit: ptr(int32(3)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Containers: []corev1.Container{
						{
							Name:    "worker",
							Image:   r.image,
							Command: []string{"buildmax-worker"},
							Args:    []string{"--task-id", task.TaskID},
							Env:     r.env,
						},
					},
				},
			},
		},
	}

	if err := r.client.CreateJob(ctx, r.namespace, job); err != nil {
		slog.Warn("scheduler: failed to create k8s Job", "task_id", task.TaskID, "job_name", jobName, "err", err)
		return "", nil, nil, err
	}
	slog.Info("scheduler: created k8s Job", "task_id", task.TaskID, "job_name", jobName, "namespace", r.namespace)
	return "k8s_job", &jobName, &createdAtUnix, nil
}

func ptr(i int32) *int32 { return &i }

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
// For use when BUILDMAX_WORKER_RUN_MODE=k8s_job. Returns (nil, nil) if not in cluster and no kubeconfig (caller can fall back or fail).
func BuildK8sJobCreator() (JobCreator, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := os.Getenv("KUBECONFIG")
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, err
		}
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return &jobCreatorImpl{clientset: clientset}, nil
}
