package watcher

import (
	"strconv"

	corev1 "k8s.io/api/core/v1"
)

// GetVolumes - service volumes
func GetVolumes(name string, secretNames []string) []corev1.Volume {
	var config0644AccessMode int32 = 0644

	vm := []corev1.Volume{
		{
			Name: "config-data",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &config0644AccessMode,
					SecretName:  name + "-config-data",
				},
			},
		},
	}

	secretConfig, _ := GetConfigSecretVolumes(secretNames)
	vm = append(vm, secretConfig...)
	return vm
}

// GetVolumeMounts - general VolumeMounts
func GetVolumeMounts(secretNames []string) []corev1.VolumeMount {

	vm := []corev1.VolumeMount{
		{
			Name:      "config-data",
			MountPath: "/var/lib/config-data/default",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/my.cnf",
			SubPath:   "my.cnf",
			ReadOnly:  true,
		},
	}

	_, secretConfig := GetConfigSecretVolumes(secretNames)
	vm = append(vm, secretConfig...)
	return vm
}

// GetConfigSecretVolumes - Returns a list of volumes associated with a list of Secret names
func GetConfigSecretVolumes(secretNames []string) ([]corev1.Volume, []corev1.VolumeMount) {
	var config0640AccessMode int32 = 0640
	secretVolumes := []corev1.Volume{}
	secretMounts := []corev1.VolumeMount{}

	for idx, secretName := range secretNames {
		secretVol := corev1.Volume{
			Name: secretName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  secretName,
					DefaultMode: &config0640AccessMode,
				},
			},
		}
		secretMount := corev1.VolumeMount{
			Name: secretName,
			// Each secret needs its own MountPath
			MountPath: "/var/lib/config-data/secret-" + strconv.Itoa(idx),
			ReadOnly:  true,
		}
		secretVolumes = append(secretVolumes, secretVol)
		secretMounts = append(secretMounts, secretMount)
	}

	return secretVolumes, secretMounts
}

// GetLogVolumeMount - Returns the VolumeMount used for logging purposes
func GetLogVolumeMount() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      LogVolume,
			MountPath: "/var/log/watcher",
			ReadOnly:  false,
		},
	}
}

// GetLogVolume - Returns the Volume used for logging purposes
func GetLogVolume() []corev1.Volume {
	return []corev1.Volume{
		{
			Name: LogVolume,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{Medium: ""},
			},
		},
	}
}

// GetKollaConfigVolumeMount - Returns the VolumeMount for the kolla config file
func GetKollaConfigVolumeMount(serviceName string) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      ConfigVolume,
		MountPath: "/var/lib/kolla/config_files/config.json",
		SubPath:   serviceName + "-config.json",
		ReadOnly:  true,
	}
}

// GetScriptVolumeMount returns the volume mount for scripts
func GetScriptVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      scriptVolume,
		MountPath: "/var/lib/openstack/bin",
		ReadOnly:  false,
	}
}

// GetScriptVolume returns the volume for scripts using the specified secret
func GetScriptVolume(secretName string) corev1.Volume {
	var scriptMode int32 = 0740
	return corev1.Volume{
		Name: scriptVolume,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				DefaultMode: &scriptMode,
				SecretName:  secretName,
			},
		},
	}
}
