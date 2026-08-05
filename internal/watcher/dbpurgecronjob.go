package watcher

import (
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/openstack-k8s-operators/lib-common/modules/common/pod"
	"github.com/openstack-k8s-operators/lib-common/modules/users"
	watcherv1beta1 "github.com/openstack-k8s-operators/watcher-operator/api/v1beta1"
)

// DBPurgeCronJob creates a CronJob for database purging operations
func DBPurgeCronJob(
	instance *watcherv1beta1.Watcher,
	labels map[string]string,
	annotations map[string]string,
) *batchv1.CronJob {

	purgeAge := fmt.Sprintf("%d", *instance.Spec.DBPurge.PurgeAge)

	// Unlike the individual Watcher services, the DbPurgeCronJob doesn't need a
	// secret that contains all of the config snippets required by every
	// service, The two snippet files that it does need (DefaultsConfigFileName
	// and CustomConfigFileName) can be extracted from the top-level watcher
	// config-data secret.

	dbPurgeVolume := []corev1.Volume{
		{
			Name: "config-data",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &config0440AccessMode,
					SecretName:  instance.Name + "-config-data",
				},
			},
		},
	}
	dbPurgeMounts := []corev1.VolumeMount{
		{
			Name:      "config-data",
			MountPath: "/etc/watcher/watcher.conf.d",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/my.cnf",
			SubPath:   "my.cnf",
			ReadOnly:  true,
		},
	}

	// Create mount for bundle CA if defined in TLS.CaBundleSecretName
	if instance.Spec.APIServiceTemplate.TLS.CaBundleSecretName != "" {
		dbPurgeVolume = append(dbPurgeVolume, instance.Spec.APIServiceTemplate.TLS.CreateVolume())
		dbPurgeMounts = append(dbPurgeMounts, instance.Spec.APIServiceTemplate.TLS.CreateVolumeMounts(nil)...)
	}

	name := instance.Name + "-db-purge"

	cron := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          *instance.Spec.DBPurge.Schedule,
			ConcurrencyPolicy: batchv1.ForbidConcurrent,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: annotations,
				},
				Spec: batchv1.JobSpec{
					Parallelism: ptr.To[int32](1),
					Completions: ptr.To[int32](1),
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy:                corev1.RestartPolicyOnFailure,
							ServiceAccountName:           instance.RbacResourceName(),
							AutomountServiceAccountToken: ptr.To(false),
							SecurityContext:              pod.RestrictivePodSecurityContext(users.WatcherUID, users.WatcherGID),
							Volumes:                      dbPurgeVolume,
							Containers: []corev1.Container{
								{
									Name: "watcher-db-manage",
									Command: []string{
										"/bin/bash", "-c",
										fmt.Sprintf("echo y | watcher-db-manage --config-dir /etc/watcher/watcher.conf.d/ --debug purge -d %s", purgeAge),
									},
									Image:           instance.Spec.APIContainerImageURL,
									SecurityContext: pod.RestrictiveSecurityContext(users.WatcherUID, users.WatcherGID),
									VolumeMounts:    dbPurgeMounts,
								},
							},
						},
					},
				},
			},
		},
	}

	if instance.Spec.NodeSelector != nil {
		cron.Spec.JobTemplate.Spec.Template.Spec.NodeSelector = *instance.Spec.NodeSelector
	}

	return cron
}
