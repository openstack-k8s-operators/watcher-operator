package watcher

import (
	"github.com/openstack-k8s-operators/lib-common/modules/common/volume"
	corev1 "k8s.io/api/core/v1"
)

var config0440AccessMode int32 = 0440

// GetVolumes - service volumes
func GetVolumes(name string, secretNames []string) []corev1.Volume {

	vm := []corev1.Volume{
		{
			Name: "config-data",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &config0440AccessMode,
					SecretName:  name + "-config-data",
				},
			},
		},
	}

	secretConfig, _ := volume.ConfigSecretVolumes(secretNames)
	vm = append(vm, secretConfig...)
	return vm
}

// GetVolumeMounts - VolumeMounts shared by every consumer of the "config-data"
// Secret (watcher-api/applier/decision-engine, plus db-sync/db-purge): the
// always-present default+global-custom config.d snippets and the mariadb
// client config, each mounted directly at its final destination via SubPath.
// The merge/staging pattern kolla used ("/var/lib/config-data/default", then
// kolla_start copying each file to its real path) is gone -- each file is
// mounted directly at its final destination from the same "config-data"
// Secret. "02-service-custom.conf" is deliberately NOT included here: it
// only exists in the per-component secrets (watcherapi/applier/decision
// -engine's own "generateServiceConfigs"), not in db-sync/db-purge's shared
// parent-CR secret, so it's added by the individual callers that need it
// instead of unconditionally here (a SubPath mount of a key the Secret
// doesn't have fails the pod outright, unlike kolla's "optional": true).
func GetVolumeMounts(secretNames []string) []corev1.VolumeMount {

	vm := []corev1.VolumeMount{
		{
			Name:      "config-data",
			MountPath: "/etc/watcher/watcher.conf.d/00-default.conf",
			SubPath:   DefaultsConfigFileName,
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/watcher/watcher.conf.d/01-global-custom.conf",
			SubPath:   GlobalCustomConfigFileName,
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/my.cnf",
			SubPath:   "my.cnf",
			ReadOnly:  true,
		},
	}

	_, secretConfig := volume.ConfigSecretVolumes(secretNames)
	vm = append(vm, secretConfig...)
	return vm
}

// GetServiceCustomVolumeMount - the "02-service-custom.conf" SubPath mount,
// used only by watcher-api/applier/decision-engine (each has its own
// "generateServiceConfigs" populating this key in their own per-component
// "config-data" Secret) -- not by db-sync/db-purge, whose shared parent-CR
// secret never has this key.
func GetServiceCustomVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      "config-data",
		MountPath: "/etc/watcher/watcher.conf.d/02-service-custom.conf",
		SubPath:   ServiceCustomConfigFileName,
		ReadOnly:  true,
	}
}
