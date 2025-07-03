/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package functional

import (
	"encoding/base64"
	"fmt"

	. "github.com/onsi/gomega" //revive:disable:dot-imports

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	watcherv1 "github.com/openstack-k8s-operators/watcher-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetDefaultWatcherSpec() map[string]interface{} {
	return map[string]interface{}{
		"databaseInstance": "openstack",
		"secret":           SecretName,
	}
}

// Second Watcher Spec to test proper parameters substitution
func GetNonDefaultWatcherSpec() map[string]interface{} {
	return map[string]interface{}{
		"apiContainerImageURL": "fake-API-Container-URL",
		"secret":               SecretName,
		"preserveJobs":         true,
		"databaseInstance":     "fakeopenstack",
		"serviceUser":          "fakeuser",
		"customServiceConfig":  "# Global config",
		"apiServiceTemplate": map[string]interface{}{
			"replicas":            2,
			"nodeSelector":        map[string]string{"foo": "bar"},
			"customServiceConfig": "# Service config",
			"tls": map[string]interface{}{
				"caBundleSecretName": "combined-ca-bundle",
			},
		},
		"prometheusSecret":         "custom-prometheus-config",
		"applierContainerImageURL": "fake-Applier-Container-URL",
		"applierServiceTemplate": map[string]interface{}{
			"replicas":            1,
			"nodeSelector":        map[string]string{"foo": "bar"},
			"customServiceConfig": "# Service config Applier",
		},
		"decisionengineContainerImageURL": "fake-DecisionEngine-Container-URL",
		"decisionengineServiceTemplate": map[string]interface{}{
			"replicas":            1,
			"nodeSelector":        map[string]string{"foo": "bar"},
			"customServiceConfig": "# Service config DecisionEngine",
		},
		"dbPurge": map[string]interface{}{
			"schedule": "1 2 * * *",
			"purgeAge": 1,
		},
	}
}

// Watcher Spec to test TLSe
func GetTLSeWatcherSpec() map[string]interface{} {
	return map[string]interface{}{
		"secret":           SecretName,
		"databaseInstance": "openstack",
		"apiServiceTemplate": map[string]interface{}{
			"tls": map[string]interface{}{
				"caBundleSecretName": "combined-ca-bundle",
				"api": map[string]interface{}{
					"internal": map[string]string{
						"secretName": "cert-watcher-internal-svc",
					},
					"public": map[string]string{
						"secretName": "cert-watcher-public-svc",
					},
				},
			},
		},
	}
}

func GetTLSIngressWatcherSpec() map[string]interface{} {
	return map[string]interface{}{
		"secret":           SecretName,
		"databaseInstance": "openstack",
		"apiServiceTemplate": map[string]interface{}{
			"tls": map[string]interface{}{
				"caBundleSecretName": "combined-ca-bundle",
			},
		},
	}
}

func GetTLSPodLevelWatcherSpec() map[string]interface{} {
	return map[string]interface{}{
		"secret":           SecretName,
		"databaseInstance": "openstack",
		"apiServiceTemplate": map[string]interface{}{
			"tls": map[string]interface{}{
				"caBundleSecretName": "combined-ca-bundle",
				"api": map[string]interface{}{
					"internal": map[string]string{
						"secretName": "cert-watcher-internal-svc",
					},
					"public": map[string]string{
						"secretName": "cert-watcher-public-svc",
					},
				},
			},
		},
	}
}

func GetDefaultWatcherAPISpec() map[string]interface{} {
	return map[string]interface{}{
		"databaseInstance":  "openstack",
		"secret":            SecretName,
		"memcachedInstance": "memcached",
		"serviceAccount":    "watcher-sa",
		"containerImage":    "test://watcher",
	}
}

func GetTLSWatcherAPISpec() map[string]interface{} {
	return map[string]interface{}{
		"databaseInstance": "openstack",
		"secret":           SecretName,
		"containerImage":   "test://watcher",
		"tls": map[string]interface{}{
			"caBundleSecretName": "combined-ca-bundle",
			"api": map[string]interface{}{
				"internal": map[string]string{
					"secretName": "cert-watcher-internal-svc",
				},
				"public": map[string]string{
					"secretName": "cert-watcher-public-svc",
				},
			},
		},
	}

}
func GetTLSCaWatcherAPISpec() map[string]interface{} {
	return map[string]interface{}{
		"databaseInstance": "openstack",
		"secret":           SecretName,
		"containerImage":   "test://watcher",
		"tls": map[string]interface{}{
			"caBundleSecretName": "combined-ca-bundle",
		},
	}
}

func GetServiceOverrideWatcherAPISpec() map[string]interface{} {
	return map[string]interface{}{
		"databaseInstance":  "openstack",
		"secret":            SecretName,
		"memcachedInstance": "memcached",
		"serviceAccount":    "watcher-sa",
		"containerImage":    "test://watcher",
		"override": map[string]interface{}{
			"service": map[string]interface{}{
				"internal": map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]string{
							"metallb.universe.tf/address-pool":    "osp-internalapi",
							"metallb.universe.tf/loadBalancerIPs": "internal-lb-ip-1,internal-lb-ip-2",
							"metallb.universe.tf/allow-shared-ip": "osp-internalapi",
						},
					},
					"spec": map[string]interface{}{
						"type": "LoadBalancer",
					},
				},
			},
		},
	}
}

func GetDefaultWatcherApplierSpec() map[string]interface{} {
	return map[string]interface{}{
		"databaseInstance":  "openstack",
		"secret":            SecretName,
		"memcachedInstance": "memcached",
		"serviceAccount":    "watcher-sa",
		"containerImage":    "test://watcher",
	}
}

func GetDefaultWatcherDecisionEngineSpec() map[string]interface{} {
	return map[string]interface{}{
		"databaseInstance":  "openstack",
		"secret":            SecretName,
		"memcachedInstance": "memcached",
		"serviceAccount":    "watcher-sa",
		"containerImage":    "test://watcher",
	}
}

func CreateWatcher(name types.NamespacedName, spec map[string]interface{}) client.Object {
	raw := map[string]interface{}{
		"apiVersion": "watcher.openstack.org/v1beta1",
		"kind":       "Watcher",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return th.CreateUnstructured(raw)
}

func GetWatcher(name types.NamespacedName) *watcherv1.Watcher {
	instance := &watcherv1.Watcher{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func WatcherConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetWatcher(name)
	return instance.Status.Conditions
}

func CreateWatcherAPI(name types.NamespacedName, spec map[string]interface{}) client.Object {
	raw := map[string]interface{}{
		"apiVersion": "watcher.openstack.org/v1beta1",
		"kind":       "WatcherAPI",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return th.CreateUnstructured(raw)
}

func GetWatcherAPI(name types.NamespacedName) *watcherv1.WatcherAPI {
	instance := &watcherv1.WatcherAPI{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func WatcherAPIConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetWatcherAPI(name)
	return instance.Status.Conditions
}

func CreateWatcherApplier(name types.NamespacedName, spec map[string]interface{}) client.Object {
	raw := map[string]interface{}{
		"apiVersion": "watcher.openstack.org/v1beta1",
		"kind":       "WatcherApplier",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return th.CreateUnstructured(raw)
}

func GetWatcherApplier(name types.NamespacedName) *watcherv1.WatcherApplier {
	instance := &watcherv1.WatcherApplier{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func WatcherApplierConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetWatcherApplier(name)
	return instance.Status.Conditions
}

func WatcherDecisionEngineConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetWatcherDecisionEngine(name)
	return instance.Status.Conditions
}

func CreateWatcherMessageBusSecret(namespace string, name string) *corev1.Secret {
	s := th.CreateSecret(
		types.NamespacedName{Namespace: namespace, Name: name},
		map[string][]byte{
			"transport_url": []byte(fmt.Sprintf("rabbit://%s/fake", name)),
		},
	)
	logger.Info("Secret created", "name", name)
	return s
}

func CreateWatcherDecisionEngine(name types.NamespacedName, spec map[string]interface{}) client.Object {
	raw := map[string]interface{}{
		"apiVersion": "watcher.openstack.org/v1beta1",
		"kind":       "WatcherDecisionEngine",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return th.CreateUnstructured(raw)
}

func GetWatcherDecisionEngine(name types.NamespacedName) *watcherv1.WatcherDecisionEngine {
	instance := &watcherv1.WatcherDecisionEngine{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func GetCronJob(name types.NamespacedName) *batchv1.CronJob {
	cron := &batchv1.CronJob{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, cron)).Should(Succeed())
	}, timeout, interval).Should(Succeed())

	return cron
}

func CreateCertSecret(name types.NamespacedName) *corev1.Secret {
	certBase64 := "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUJlekNDQVNLZ0F3SUJBZ0lRTkhER1lzQnM3OThpYkREN3EvbzJsakFLQmdncWhrak9QUVFEQWpBZU1Sd3cKR2dZRFZRUURFeE55YjI5MFkyRXRhM1YwZEd3dGNIVmliR2xqTUI0WERUSTBNREV4TlRFd01UVXpObG9YRFRNMApNREV4TWpFd01UVXpObG93SGpFY01Cb0dBMVVFQXhNVGNtOXZkR05oTFd0MWRIUnNMWEIxWW14cFl6QlpNQk1HCkJ5cUdTTTQ5QWdFR0NDcUdTTTQ5QXdFSEEwSUFCRDc4YXZYcWhyaEM1dzhzOVdrZDRJcGJlRXUwM0NSK1hYVWQKa0R6T1J5eGE5d2NjSWREaXZiR0pqSkZaVFRjVm1ianExQk1Zc2pyMTJVSUU1RVQzVmxxalFqQkFNQTRHQTFVZApEd0VCL3dRRUF3SUNwREFQQmdOVkhSTUJBZjhFQlRBREFRSC9NQjBHQTFVZERnUVdCQlRLSml6V1VKOWVVS2kxCmRzMGxyNmM2c0Q3RUJEQUtCZ2dxaGtqT1BRUURBZ05IQURCRUFpQklad1lxNjFCcU1KYUI2VWNGb1JzeGVjd0gKNXovek1PZHJPeWUwbU5pOEpnSWdRTEI0d0RLcnBmOXRYMmxvTSswdVRvcEFEU1lJbnJjZlZ1NEZCdVlVM0lnPQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg=="
	keyBase64 := "LS0tLS1CRUdJTiBFQyBQUklWQVRFIEtFWS0tLS0tCk1IY0NBUUVFSUptbGNLUEl1RitFc3RhYkxnVmowZkNhdzFTK09xNnJPU3M0U3pMQkJGYVFvQW9HQ0NxR1NNNDkKQXdFSG9VUURRZ0FFUHZ4cTllcUd1RUxuRHl6MWFSM2dpbHQ0UzdUY0pINWRkUjJRUE01SExGcjNCeHdoME9LOQpzWW1Na1ZsTk54V1p1T3JVRXhpeU92WFpRZ1RrUlBkV1dnPT0KLS0tLS1FTkQgRUMgUFJJVkFURSBLRVktLS0tLQo=="

	cert, _ := base64.StdEncoding.DecodeString(certBase64)
	key, _ := base64.StdEncoding.DecodeString(keyBase64)

	s := &corev1.Secret{}
	Eventually(func(_ Gomega) {
		s = th.CreateSecret(
			name,
			map[string][]byte{
				"ca.crt":  []byte(cert),
				"tls.crt": []byte(cert),
				"tls.key": []byte(key),
			})
	}, timeout, interval).Should(Succeed())

	return s
}

// GetSampleTopologySpec - An opinionated Topology Spec sample used to
// test Watcher components. It returns both the user input representation
// in the form of map[string]string, and the Golang expected representation
// used in the test asserts.
func GetSampleTopologySpec(label string) (map[string]interface{}, []corev1.TopologySpreadConstraint) {
	// Build the topology Spec yaml representation
	topologySpec := map[string]interface{}{
		"topologySpreadConstraints": []map[string]interface{}{
			{
				"maxSkew":           1,
				"topologyKey":       corev1.LabelHostname,
				"whenUnsatisfiable": "ScheduleAnyway",
				"labelSelector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"service": label,
					},
				},
			},
		},
	}
	// Build the topologyObj representation
	topologySpecObj := []corev1.TopologySpreadConstraint{
		{
			MaxSkew:           1,
			TopologyKey:       corev1.LabelHostname,
			WhenUnsatisfiable: corev1.ScheduleAnyway,
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"service": label,
				},
			},
		},
	}
	return topologySpec, topologySpecObj
}
