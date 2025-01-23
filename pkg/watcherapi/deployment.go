package watcherapi

import (
	"path/filepath"

	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	watcherv1beta1 "github.com/openstack-k8s-operators/watcher-operator/api/v1beta1"
	watcher "github.com/openstack-k8s-operators/watcher-operator/pkg/watcher"
)

const (
	// ServiceCommand -
	ServiceCommand = "/usr/local/bin/kolla_start"
)

// Deployment - returns a WatcherAPI Deployment
func Deployment(
	instance *watcherv1beta1.WatcherAPI,
	configHash string,
	prometheusCaCertSecret map[string]string,
	labels map[string]string,
) (*appsv1.Deployment, error) {

	runAsUser := int64(0)
	var config0644AccessMode int32 = 0644
	envVars := map[string]env.Setter{}
	envVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	envVars["CONFIG_HASH"] = env.SetValue(configHash)
	livenessProbe := &corev1.Probe{
		TimeoutSeconds:      5,
		PeriodSeconds:       3,
		InitialDelaySeconds: 5,
	}
	readinessProbe := &corev1.Probe{
		TimeoutSeconds:      5,
		PeriodSeconds:       5,
		InitialDelaySeconds: 5,
	}
	args := []string{"-c", ServiceCommand}

	//
	// https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/
	//
	livenessProbe.HTTPGet = &corev1.HTTPGetAction{
		Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(watcher.WatcherPublicPort)},
	}
	readinessProbe.HTTPGet = livenessProbe.HTTPGet

	apiVolumes := append(watcher.GetLogVolume(),
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

	apiVolumeMounts := []corev1.VolumeMount{
		watcher.GetKollaConfigVolumeMount("watcher-api"),
	}
	apiVolumeMounts = append(apiVolumeMounts, watcher.GetLogVolumeMount()...)

	// Create mount for bundle CA if defined in TLS.CaBundleSecretName
	if instance.Spec.TLS.CaBundleSecretName != "" {
		apiVolumes = append(apiVolumes, instance.Spec.TLS.CreateVolume())
		apiVolumeMounts = append(apiVolumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
	}

	if len(prometheusCaCertSecret) != 0 {
		apiVolumes = append(apiVolumes,
			corev1.Volume{
				Name: "custom-prometheus-ca",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: prometheusCaCertSecret["casecret_name"],
					},
				},
			},
		)
		apiVolumeMounts = append(apiVolumeMounts,
			corev1.VolumeMount{
				Name:      "custom-prometheus-ca",
				MountPath: filepath.Join(watcher.PrometheusCaCertFolderPath, prometheusCaCertSecret["casecret_key"]),
				SubPath:   prometheusCaCertSecret["casecret_key"],
				ReadOnly:  true,
			},
		)
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Replicas: instance.Spec.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: instance.Spec.ServiceAccount,
					Containers: []corev1.Container{
						{
							Name: instance.Name + "-log",
							Command: []string{
								"/usr/bin/dumb-init",
							},
							Args: []string{
								"--single-child",
								"--",
								"/usr/bin/tail",
								"-n+1",
								"-F",
								watcher.WatcherLogPath + instance.Name + ".log",
							},
							Image: instance.Spec.ContainerImage,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &runAsUser,
							},
							Env:            env.MergeEnvs([]corev1.EnvVar{}, envVars),
							VolumeMounts:   watcher.GetLogVolumeMount(),
							Resources:      instance.Spec.Resources,
							ReadinessProbe: readinessProbe,
							LivenessProbe:  livenessProbe,
						},
						{
							Name: watcher.ServiceName + "-api",
							Command: []string{
								"/bin/bash",
							},
							Args:  args,
							Image: instance.Spec.ContainerImage,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &runAsUser,
							},
							Env: env.MergeEnvs([]corev1.EnvVar{}, envVars),
							VolumeMounts: append(watcher.GetVolumeMounts(
								[]string{}),
								apiVolumeMounts...,
							),
							Resources:      instance.Spec.Resources,
							ReadinessProbe: readinessProbe,
							LivenessProbe:  livenessProbe,
						},
					},
				},
			},
		},
	}

	deployment.Spec.Template.Spec.Volumes = append(watcher.GetVolumes(
		instance.Name,
		[]string{}),
		apiVolumes...)

	if instance.Spec.NodeSelector != nil {
		deployment.Spec.Template.Spec.NodeSelector = *instance.Spec.NodeSelector
	}
	return deployment, nil
}
