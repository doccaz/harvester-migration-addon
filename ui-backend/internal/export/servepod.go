// servepod.go
//
// The pod that serves one finished OVA (see serve.go), and the ensure/reap
// logic around it.
package export

import (
	"context"
	"fmt"
	"strconv"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"
)

const (
	// exportLabelServe marks a serve pod. Like the cleanup label it must NOT carry
	// exportLabelMarker, which is how exports are enumerated.
	exportLabelServe = "vm-import-ui.harvesterhci.io/export-serve"

	serveActiveDeadline = int64(4 * 60 * 60)
	serveIdleSeconds    = 30 * 60
)

func exportServePodName(exportID string) string { return "vm-export-serve-" + exportID }

// buildServePod renders the serve pod for the finished export job. The export
// volume is mounted read-only. The pod is owned by the export Job, so deleting
// the export (and its purge) removes it too.
func buildServePod(job *batchv1.Job, cfg exportConfig, token string) (*corev1.Pod, error) {
	id := job.Labels[exportLabelID]
	if !exportIDPattern.MatchString(id) {
		return nil, fmt.Errorf("export job %s has no valid export id", job.Name)
	}
	target := job.Annotations[exportAnnTargetName]
	if target == "" {
		return nil, fmt.Errorf("export %s has no recorded target file", id)
	}
	if cfg.Image == "" {
		return nil, fmt.Errorf("no image configured for the serve pod (EXPORT_IMAGE)")
	}
	if _, err := safeExportPath(exportMountPath, target+".ova"); err != nil {
		return nil, err
	}
	// Use the claim the export itself wrote to.
	claim := cfg.PVC
	for _, v := range job.Spec.Template.Spec.Volumes {
		if v.Name == "export" && v.PersistentVolumeClaim != nil {
			claim = v.PersistentVolumeClaim.ClaimName
		}
	}
	if claim == "" {
		return nil, fmt.Errorf("no export storage configured; set export.storage in the chart values")
	}

	automount := false
	deadline := serveActiveDeadline
	labels := map[string]string{
		exportLabelManagedBy: exportManagedByValue,
		exportLabelServe:     id,
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      exportServePodName(id),
			Namespace: job.Namespace,
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "batch/v1", Kind: "Job", Name: job.Name, UID: job.UID,
			}},
		},
		Spec: corev1.PodSpec{
			RestartPolicy:                corev1.RestartPolicyNever,
			AutomountServiceAccountToken: &automount,
			ActiveDeadlineSeconds:        &deadline,
			SecurityContext: &corev1.PodSecurityContext{
				RunAsUser: cfg.RunAsUser,
				FSGroup:   cfg.FSGroup,
			},
			Volumes: []corev1.Volume{{
				Name: "export",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: claim, ReadOnly: true},
				},
			}},
			Containers: []corev1.Container{{
				Name:    "serve",
				Image:   cfg.Image,
				Command: []string{BinaryPath, ServeArg},
				Env: []corev1.EnvVar{
					{Name: "EXPORT_ROOT", Value: exportMountPath},
					{Name: "EXPORT_TARGET", Value: target},
					{Name: "EXPORT_SERVE_TOKEN", Value: token},
					{Name: "EXPORT_SERVE_IDLE_SECONDS", Value: strconv.Itoa(serveIdleSeconds)},
				},
				Ports:        []corev1.ContainerPort{{Name: "serve", ContainerPort: ServePort}},
				VolumeMounts: []corev1.VolumeMount{{Name: "export", MountPath: exportMountPath, ReadOnly: true}},
				ReadinessProbe: &corev1.Probe{
					ProbeHandler:  corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt32(ServePort)}},
					PeriodSeconds: 2,
				},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("50m"),
						corev1.ResourceMemory: resource.MustParse("64Mi"),
					},
					Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("512Mi")},
				},
			}},
		},
	}, nil
}

// serveState is what ensureServePod found.
type serveState struct {
	Ready bool
	IP    string
}

// ensureServePod makes sure a serve pod for the export exists and reports
// whether it is ready. A pod that already exited (idle timeout, deadline) is
// removed so the next call creates a fresh one. It acts with the caller's
// clients, so a user who may not create pods gets Kubernetes' own refusal.
func ensureServePod(ctx context.Context, clients *kube.Clients, job *batchv1.Job, cfg exportConfig, key []byte) (serveState, error) {
	pods := clients.Clientset.CoreV1().Pods(job.Namespace)
	id := job.Labels[exportLabelID]
	pod, err := pods.Get(ctx, exportServePodName(id), metav1.GetOptions{})
	if errors.IsNotFound(err) {
		want, berr := buildServePod(job, cfg, serveToken(key, job.Namespace, id))
		if berr != nil {
			return serveState{}, berr
		}
		if _, cerr := pods.Create(ctx, want, metav1.CreateOptions{}); cerr != nil && !errors.IsAlreadyExists(cerr) {
			return serveState{}, cerr
		}
		return serveState{}, nil
	}
	if err != nil {
		return serveState{}, err
	}
	if pod.DeletionTimestamp != nil {
		return serveState{}, nil
	}
	switch pod.Status.Phase {
	case corev1.PodSucceeded, corev1.PodFailed:
		if derr := pods.Delete(ctx, pod.Name, metav1.DeleteOptions{}); derr != nil && !errors.IsNotFound(derr) {
			return serveState{}, derr
		}
		return serveState{}, nil
	case corev1.PodRunning:
		if pod.Status.PodIP != "" && podReady(pod) {
			return serveState{Ready: true, IP: pod.Status.PodIP}, nil
		}
	}
	return serveState{}, nil
}

func podReady(p *corev1.Pod) bool {
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}
