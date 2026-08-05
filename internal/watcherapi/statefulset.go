// Package watcherapi provides functionality for managing Watcher API StatefulSet resources
package watcherapi

import (
	"fmt"
	"path/filepath"

	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/affinity"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/pod"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	"github.com/openstack-k8s-operators/lib-common/modules/common/volume"
	"github.com/openstack-k8s-operators/lib-common/modules/users"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	watcherv1beta1 "github.com/openstack-k8s-operators/watcher-operator/api/v1beta1"
	watcher "github.com/openstack-k8s-operators/watcher-operator/internal/watcher"
)

// StatefulSet - returns a WatcherAPI StatefulSet
func StatefulSet(
	instance *watcherv1beta1.WatcherAPI,
	configHash string,
	prometheusCaCertSecret map[string]string,
	labels map[string]string,
	topology *topologyv1.Topology,
	memcached *memcachedv1.Memcached,
) (*appsv1.StatefulSet, error) {

	envVars := map[string]env.Setter{}
	envVars["CONFIG_HASH"] = env.SetValue(configHash)
	// This allows the pod to start up slowly. The pod will only be killed
	// if it does not succeed a probe in 60 seconds.
	startupProbe := &corev1.Probe{
		FailureThreshold: 6,
		PeriodSeconds:    10,
	}
	livenessProbe := &corev1.Probe{
		TimeoutSeconds: 5,
		PeriodSeconds:  5,
	}
	readinessProbe := &corev1.Probe{
		TimeoutSeconds: 5,
		PeriodSeconds:  5,
	}
	args := []string{"-DFOREGROUND"}

	//
	// https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/
	//
	livenessProbe.HTTPGet = &corev1.HTTPGetAction{
		Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(watcher.WatcherPublicPort)},
	}
	readinessProbe.HTTPGet = livenessProbe.HTTPGet
	startupProbe.HTTPGet = livenessProbe.HTTPGet

	if instance.Spec.TLS.API.Enabled(service.EndpointPublic) {
		livenessProbe.HTTPGet.Scheme = corev1.URISchemeHTTPS
		readinessProbe.HTTPGet.Scheme = corev1.URISchemeHTTPS
		startupProbe.HTTPGet.Scheme = corev1.URISchemeHTTPS
	}

	apiVolumes := []corev1.Volume{
		volume.WritableDirVolume(watcher.LogVolume),
		volume.WritableDirVolume(volume.RunHttpdVolumeName),
		volume.WritableDirVolume(volume.VarLogHttpdVolumeName),
	}

	// httpd-specific config, mounted directly at their final destinations
	// from the same per-component "config-data" Secret every other mount
	// below also draws from -- watcher-blank.conf/02-service-custom.conf
	// are only needed here (not by applier/decision-engine), and
	// httpd.conf/10-watcher-wsgi-main.conf/ssl.conf only exist in this
	// component's own secret in the first place (scanned automatically
	// from templates/watcherapi/config/, plus ssl.conf via CommonTemplates).
	apiVolumeMounts := []corev1.VolumeMount{
		{
			Name:      "config-data",
			MountPath: "/etc/watcher/watcher.conf",
			SubPath:   "watcher-blank.conf",
			ReadOnly:  true,
		},
		watcher.GetServiceCustomVolumeMount(),
		{
			Name:      "config-data",
			MountPath: "/etc/httpd/conf/httpd.conf",
			SubPath:   "httpd.conf",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/httpd/conf.d/10-watcher-wsgi-main.conf",
			SubPath:   "10-watcher-wsgi-main.conf",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/httpd/conf.d/ssl.conf",
			SubPath:   "ssl.conf",
			ReadOnly:  true,
		},
		volume.WritableDirVolumeMount(volume.RunHttpdVolumeName, volume.RunHttpdMountPath),
		volume.WritableDirVolumeMount(volume.VarLogHttpdVolumeName, volume.VarLogHttpdMountPath),
	}
	apiVolumeMounts = append(apiVolumeMounts, volume.WritableDirVolumeMount(watcher.LogVolume, "/var/log/watcher"))

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
	for _, endpt := range []service.Endpoint{service.EndpointInternal, service.EndpointPublic} {
		if instance.Spec.TLS.API.Enabled(endpt) {
			var tlsEndptCfg tls.GenericService
			switch endpt {
			case service.EndpointPublic:
				tlsEndptCfg = instance.Spec.TLS.API.Public
			case service.EndpointInternal:
				tlsEndptCfg = instance.Spec.TLS.API.Internal
			}

			svc, err := tlsEndptCfg.ToService()
			if err != nil {
				return nil, err
			}
			// Final paths, matching what 10-watcher-wsgi-main.conf's
			// SSLCertificateFile/SSLCertificateKeyFile are rendered with
			// (watcherapi_controller.go) -- without this, CreateVolumeMounts
			// defaults to lib-common's staging path, which nothing copies
			// from once kolla's config.json is gone.
			certMount := fmt.Sprintf("/etc/pki/tls/certs/%s.crt", endpt.String())
			keyMount := fmt.Sprintf("/etc/pki/tls/private/%s.key", endpt.String())
			svc.CertMount = &certMount
			svc.KeyMount = &keyMount
			apiVolumes = append(apiVolumes, svc.CreateVolume(endpt.String()))
			apiVolumeMounts = append(apiVolumeMounts, svc.CreateVolumeMounts(endpt.String())...)
		}
	}

	// add MTLS cert if defined
	if memcached.Status.MTLSCert != "" {
		certMountPath := memcachedv1.CertPathDst
		keyMountPath := memcachedv1.KeyPathDst
		apiVolumes = append(apiVolumes, memcached.CreateMTLSVolume())
		apiVolumeMounts = append(apiVolumeMounts, memcached.CreateMTLSVolumeMounts(&certMountPath, &keyMountPath)...)
	}

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			PodManagementPolicy: appsv1.ParallelPodManagement,
			Replicas:            instance.Spec.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName:           instance.Spec.ServiceAccount,
					AutomountServiceAccountToken: ptr.To(false),
					// httpd.conf's User/Group were changed from apache to
					// watcher (matching 10-watcher-wsgi-main.conf's
					// pre-existing WSGIDaemonProcess user=watcher
					// group=watcher, unchanged by this migration -- proof a
					// dedicated "watcher" system user already exists in the
					// image, same evidence keystone/heat/cinder/manila had).
					// apache is still granted as a supplemental group here,
					// matching keystone-operator's identical fix, so httpd
					// can read RPM-shipped conf.d files baked into the image
					// with restrictive apache-group ownership.
					SecurityContext: pod.RestrictivePodSecurityContext(users.WatcherUID, users.WatcherGID, users.ApacheGID),
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
							Image:           instance.Spec.ContainerImage,
							SecurityContext: pod.RestrictiveSecurityContext(users.WatcherUID, users.WatcherGID),
							Env:             env.MergeEnvs([]corev1.EnvVar{}, envVars),
							VolumeMounts:    []corev1.VolumeMount{volume.WritableDirVolumeMount(watcher.LogVolume, "/var/log/watcher")},
							Resources:       instance.Spec.Resources,
							ReadinessProbe:  readinessProbe,
							LivenessProbe:   livenessProbe,
							StartupProbe:    startupProbe,
						},
						{
							Name: watcher.ServiceName + "-api",
							Command: []string{
								"/usr/sbin/httpd",
							},
							Args:            args,
							Image:           instance.Spec.ContainerImage,
							SecurityContext: pod.RestrictiveSecurityContext(users.WatcherUID, users.WatcherGID),
							Env:             env.MergeEnvs([]corev1.EnvVar{}, envVars),
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

	statefulSet.Spec.Template.Spec.Volumes = append(watcher.GetVolumes(
		instance.Name,
		[]string{}),
		apiVolumes...)

	if instance.Spec.NodeSelector != nil {
		statefulSet.Spec.Template.Spec.NodeSelector = *instance.Spec.NodeSelector
	}

	if topology != nil {
		topology.ApplyTo(&statefulSet.Spec.Template)
	} else {
		// If possible two pods of the same service should not
		// run on the same worker node. If this is not possible
		// the get still created on the same worker node.
		statefulSet.Spec.Template.Spec.Affinity = affinity.DistributePods(
			common.AppSelector,
			[]string{
				instance.Name,
			},
			corev1.LabelHostname,
		)
	}

	return statefulSet, nil
}
