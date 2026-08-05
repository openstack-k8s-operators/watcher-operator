// Package watcherdecisionengine provides functionality for managing Watcher Decision Engine StatefulSet resources
package watcherdecisionengine

import (
	"path/filepath"

	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/affinity"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/pod"
	"github.com/openstack-k8s-operators/lib-common/modules/common/volume"
	"github.com/openstack-k8s-operators/lib-common/modules/users"
	watcherv1beta1 "github.com/openstack-k8s-operators/watcher-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/watcher-operator/internal/watcher"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

const (
	// ComponentName -
	ComponentName = watcher.ServiceName + "-decision-engine"
)

// StatefulSet creates a StatefulSet for the Watcher Decision Engine component
func StatefulSet(
	instance *watcherv1beta1.WatcherDecisionEngine,
	configHash string,
	prometheusCaCertSecret map[string]string,
	labels map[string]string,
	topology *topologyv1.Topology,
	memcached *memcachedv1.Memcached,
) *appsv1.StatefulSet {

	// This allows the pod to start up slowly. The pod will only be killed
	// if it does not succeed a probe in 60 seconds.
	startupProbe := &corev1.Probe{
		FailureThreshold: 6,
		PeriodSeconds:    10,
	}
	// After the first successful startupProbe, livenessProbe takes over
	livenessProbe := &corev1.Probe{
		// TODO might need tuning
		TimeoutSeconds: 30,
		PeriodSeconds:  30,
	}
	readinessProbe := &corev1.Probe{
		// TODO might need tuning
		TimeoutSeconds: 30,
		PeriodSeconds:  30,
	}

	envVars := map[string]env.Setter{}
	envVars["CONFIG_HASH"] = env.SetValue(configHash)

	startupProbe.Exec = &corev1.ExecAction{
		Command: []string{
			"/usr/bin/pgrep", "-f", "-r", "DRST", ComponentName,
		},
	}

	livenessProbe.Exec = &corev1.ExecAction{
		Command: []string{
			"/usr/bin/pgrep", "-f", "-r", "DRST", ComponentName,
		},
	}

	readinessProbe.Exec = &corev1.ExecAction{
		Command: []string{
			"/usr/bin/pgrep", "-f", "-r", "DRST", ComponentName,
		},
	}
	volumes := []corev1.Volume{volume.WritableDirVolume(watcher.LogVolume)}

	volumeMounts := []corev1.VolumeMount{
		watcher.GetServiceCustomVolumeMount(),
		volume.WritableDirVolumeMount(watcher.LogVolume, "/var/log/watcher"),
	}

	// Create mount for bundle CA if defined in TLS.CaBundleSecretName
	if instance.Spec.TLS.CaBundleSecretName != "" {
		volumes = append(volumes, instance.Spec.TLS.CreateVolume())
		volumeMounts = append(volumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
	}

	if len(prometheusCaCertSecret) != 0 {
		volumes = append(volumes,
			corev1.Volume{
				Name: "custom-prometheus-ca",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: prometheusCaCertSecret["casecret_name"],
					},
				},
			},
		)
		volumeMounts = append(volumeMounts,
			corev1.VolumeMount{
				Name:      "custom-prometheus-ca",
				MountPath: filepath.Join(watcher.PrometheusCaCertFolderPath, prometheusCaCertSecret["casecret_key"]),
				SubPath:   prometheusCaCertSecret["casecret_key"],
				ReadOnly:  true,
			},
		)
	}

	// add MTLS cert if defined
	if memcached.Status.MTLSCert != "" {
		certMountPath := memcachedv1.CertPathDst
		keyMountPath := memcachedv1.KeyPathDst
		volumes = append(volumes, memcached.CreateMTLSVolume())
		volumeMounts = append(volumeMounts, memcached.CreateMTLSVolumeMounts(&certMountPath, &keyMountPath)...)
	}

	statefulset := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Replicas: instance.Spec.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName:           instance.Spec.ServiceAccount,
					AutomountServiceAccountToken: ptr.To(false),
					SecurityContext:              pod.RestrictivePodSecurityContext(users.WatcherUID, users.WatcherGID),
					Containers: []corev1.Container{
						{
							Name: ComponentName,
							Command: []string{
								"/usr/bin/watcher-decision-engine",
							},
							Args:            []string{"--config-dir", "/etc/watcher/watcher.conf.d"},
							Image:           instance.Spec.ContainerImage,
							SecurityContext: pod.RestrictiveSecurityContext(users.WatcherUID, users.WatcherGID),
							Env:             env.MergeEnvs([]corev1.EnvVar{}, envVars),
							VolumeMounts: append(watcher.GetVolumeMounts(
								[]string{}),
								volumeMounts...,
							),
							Resources:      instance.Spec.Resources,
							StartupProbe:   startupProbe,
							ReadinessProbe: readinessProbe,
							LivenessProbe:  livenessProbe,
						},
					},
				},
			},
		},
	}

	if topology != nil {
		topology.ApplyTo(&statefulset.Spec.Template)
	} else {
		// If possible two pods of the same service should not
		// run on the same worker node. If this is not possible
		// the get still created on the same worker node.
		statefulset.Spec.Template.Spec.Affinity = affinity.DistributePods(
			common.AppSelector,
			[]string{
				instance.Name,
			},
			corev1.LabelHostname,
		)
	}

	statefulset.Spec.Template.Spec.Volumes = append(watcher.GetVolumes(
		instance.Name,
		[]string{}),
		volumes...)

	if instance.Spec.NodeSelector != nil {
		statefulset.Spec.Template.Spec.NodeSelector = *instance.Spec.NodeSelector
	}

	return statefulset

}
