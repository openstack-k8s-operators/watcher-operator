package watcher

import (
	watcherv1beta1 "github.com/openstack-k8s-operators/watcher-operator/api/v1beta1"

	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/pod"
	"github.com/openstack-k8s-operators/lib-common/modules/users"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

// DbSyncJob func
func DbSyncJob(instance *watcherv1beta1.Watcher, labels map[string]string, annotations map[string]string) *batchv1.Job {
	// Unlike the individual Watcher services, the DbSyncJob doesn't need a
	// secret that contains all of the config snippets required by every
	// service, The two snippet files that it does need (DefaultsConfigFileName
	// and CustomConfigFileName) can be extracted from the top-level watcher
	// config-data secret.
	dbSyncVolume := []corev1.Volume{
		{
			Name: "db-sync-config-data",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &config0440AccessMode,
					SecretName:  instance.Name + "-config-data",
					Items: []corev1.KeyToPath{
						{
							Key:  DefaultsConfigFileName,
							Path: DefaultsConfigFileName,
						},
						{
							Key:  "my.cnf",
							Path: "my.cnf",
						},
					},
				},
			},
		},
	}

	dbSyncMounts := []corev1.VolumeMount{
		{
			Name:      "db-sync-config-data",
			MountPath: "/etc/watcher/watcher.conf.d",
			ReadOnly:  true,
		},
		{
			Name:      "db-sync-config-data",
			MountPath: "/etc/my.cnf",
			SubPath:   "my.cnf",
			ReadOnly:  true,
		},
	}

	// Create mount for bundle CA if defined in TLS.CaBundleSecretName
	if instance.Spec.APIServiceTemplate.TLS.CaBundleSecretName != "" {
		dbSyncVolume = append(dbSyncVolume, instance.Spec.APIServiceTemplate.TLS.CreateVolume())
		dbSyncMounts = append(dbSyncMounts, instance.Spec.APIServiceTemplate.TLS.CreateVolumeMounts(nil)...)
	}

	args := []string{"upgrade"}

	envVars := map[string]env.Setter{}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-db-sync",
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: annotations,
				},
				Spec: corev1.PodSpec{
					RestartPolicy:                corev1.RestartPolicyOnFailure,
					ServiceAccountName:           instance.RbacResourceName(),
					AutomountServiceAccountToken: ptr.To(false),
					SecurityContext:              pod.RestrictivePodSecurityContext(users.WatcherUID, users.WatcherGID),
					Containers: []corev1.Container{
						{
							Name: instance.Name + "-db-sync",
							Command: []string{
								"watcher-db-manage",
							},
							Args:            args,
							Image:           instance.Spec.APIContainerImageURL,
							SecurityContext: pod.RestrictiveSecurityContext(users.WatcherUID, users.WatcherGID),
							Env:             env.MergeEnvs([]corev1.EnvVar{}, envVars),
							VolumeMounts:    dbSyncMounts,
						},
					},
				},
			},
		},
	}

	job.Spec.Template.Spec.Volumes = dbSyncVolume

	return job
}
