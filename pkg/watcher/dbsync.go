package watcher

import (
	watcherv1beta1 "github.com/openstack-k8s-operators/watcher-operator/api/v1beta1"

	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// DBSyncCommand -
	DBSyncCommand = "/usr/local/bin/kolla_set_configs && /usr/local/bin/kolla_start"
)

// DbSyncJob func
func DbSyncJob(instance *watcherv1beta1.Watcher, labels map[string]string, annotations map[string]string) *batchv1.Job {
	secretNames := []string{}
	var config0644AccessMode int32 = 0644

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
					DefaultMode: &config0644AccessMode,
					SecretName:  instance.Name + "-config-data",
					Items: []corev1.KeyToPath{
						{
							Key:  DefaultsConfigFileName,
							Path: DefaultsConfigFileName,
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
			Name:      "config-data",
			MountPath: "/var/lib/kolla/config_files/config.json",
			SubPath:   "watcher-dbsync-config.json",
			ReadOnly:  true,
		},
	}

	// Create mount for bundle CA if defined in TLS.CaBundleSecretName
	if instance.Spec.TLS.CaBundleSecretName != "" {
		dbSyncVolume = append(dbSyncVolume, instance.Spec.TLS.CreateVolume())
		dbSyncMounts = append(dbSyncMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
	}

	args := []string{"-c", DBSyncCommand}

	runAsUser := int64(0)
	envVars := map[string]env.Setter{}
	envVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	envVars["KOLLA_BOOTSTRAP"] = env.SetValue("TRUE")

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
					RestartPolicy:      corev1.RestartPolicyOnFailure,
					ServiceAccountName: instance.RbacResourceName(),
					Containers: []corev1.Container{
						{
							Name: instance.Name + "-db-sync",
							Command: []string{
								"/bin/bash",
							},
							Args:  args,
							Image: instance.Spec.APIContainerImageURL,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &runAsUser,
							},
							Env: env.MergeEnvs([]corev1.EnvVar{}, envVars),
							VolumeMounts: append(GetVolumeMounts(secretNames),
								dbSyncMounts...),
						},
					},
				},
			},
		},
	}

	job.Spec.Template.Spec.Volumes = append(GetVolumes(
		instance.Name,
		secretNames),
		dbSyncVolume...,
	)

	return job
}
