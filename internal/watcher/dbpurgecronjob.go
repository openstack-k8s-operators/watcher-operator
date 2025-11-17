package watcher

import (
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	watcherv1beta1 "github.com/openstack-k8s-operators/watcher-operator/api/v1beta1"
)

const (
	// ServiceCommand is the command used to start the service
	ServiceCommand = "/usr/local/bin/kolla_start"
)

// DBPurgeCronJob creates a CronJob for database purging operations
func DBPurgeCronJob(
	instance *watcherv1beta1.Watcher,
	labels map[string]string,
	annotations map[string]string,
) *batchv1.CronJob {

	var config0644AccessMode int32 = 0644
	args := []string{"-c", ServiceCommand}

	envVars := map[string]env.Setter{}
	envVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	envVars["KOLLA_BOOTSTRAP"] = env.SetValue("true")

	envVars["PURGE_AGE"] = env.SetValue(fmt.Sprintf("%d", *instance.Spec.DBPurge.PurgeAge))

	env := env.MergeEnvs([]corev1.EnvVar{}, envVars)

	// Unlike the individual Watcher services, the DbPurgeCronJob doesn't need a
	// secret that contains all of the config snippets required by every
	// service, The two snippet files that it does need (DefaultsConfigFileName
	// and CustomConfigFileName) can be extracted from the top-level watcher
	// config-data secret.

	dbPurgeVolume := append(GetVolumes(instance.Name, []string{}),
		corev1.Volume{
			Name: "config-data-custom",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &config0644AccessMode,
					SecretName:  instance.Name + "-config-data",
				},
			},
		},
	)

	dbPurgeMounts := []corev1.VolumeMount{
		GetKollaConfigVolumeMount("watcher-dbpurge"),
	}

	dbPurgeMounts = append(GetVolumeMounts(
		[]string{}),
		dbPurgeMounts...,
	)

	// Add the script volume
	dbPurgeVolume = append(dbPurgeVolume, GetScriptVolume(instance.Name+"-scripts"))
	dbPurgeMounts = append(dbPurgeMounts, GetScriptVolumeMount())

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
							RestartPolicy:      corev1.RestartPolicyOnFailure,
							ServiceAccountName: instance.RbacResourceName(),
							Volumes:            dbPurgeVolume,
							Containers: []corev1.Container{
								{
									Name: "watcher-db-manage",
									Command: []string{
										"/bin/bash",
									},
									Args:  args,
									Image: instance.Spec.APIContainerImageURL,
									SecurityContext: &corev1.SecurityContext{
										RunAsUser: ptr.To(WatcherUserID),
									},
									Env:          env,
									VolumeMounts: dbPurgeMounts,
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
