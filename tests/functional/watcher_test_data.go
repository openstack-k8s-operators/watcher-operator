/*
Copyright 2024.

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

// Package functional implements the envTest coverage for watcher-operator
package functional

import (
	"fmt"

	"github.com/openstack-k8s-operators/watcher-operator/pkg/watcher"

	"k8s.io/apimachinery/pkg/types"
)

type APIType string

// WatcherTestData is the data structure used to provide input data to envTest
type WatcherTestData struct {
	//DatabaseHostname             string
	DatabaseInstance                  string
	RabbitMqClusterName               string
	Instance                          types.NamespacedName
	Watcher                           types.NamespacedName
	WatcherDatabaseName               types.NamespacedName
	WatcherDatabaseAccount            types.NamespacedName
	WatcherDatabaseAccountSecret      types.NamespacedName
	InternalTopLevelSecretName        types.NamespacedName
	PrometheusSecretName              types.NamespacedName
	WatcherTransportURL               types.NamespacedName
	KeystoneServiceName               types.NamespacedName
	WatcherAPI                        types.NamespacedName
	WatcherAPIConfigSecret            types.NamespacedName
	MemcachedNamespace                types.NamespacedName
	ServiceAccountName                types.NamespacedName
	RoleName                          types.NamespacedName
	RoleBindingName                   types.NamespacedName
	WatcherDBSync                     types.NamespacedName
	WatcherAPIStatefulSet             types.NamespacedName
	WatcherDecisionEngine             types.NamespacedName
	WatcherDecisionEngineConfigSecret types.NamespacedName
	WatcherDecisionEngineStatefulSet  types.NamespacedName
	WatcherDecisionEngineSecret       types.NamespacedName
	WatcherPublicServiceName          types.NamespacedName
	WatcherInternalServiceName        types.NamespacedName
	WatcherKeystoneEndpointName       types.NamespacedName
	WatcherApplier                    types.NamespacedName
	WatcherApplierConfigSecret        types.NamespacedName
	WatcherApplierStatefulSet         types.NamespacedName
	WatcherPublicCertSecret           types.NamespacedName
	WatcherInternalCertSecret         types.NamespacedName
	WatcherApplierSecret              types.NamespacedName
}

// GetWatcherTestData is a function that initialize the WatcherTestData
// used in the test
func GetWatcherTestData(watcherName types.NamespacedName) WatcherTestData {
	m := watcherName
	return WatcherTestData{
		Instance: m,

		Watcher: types.NamespacedName{
			Namespace: watcherName.Namespace,
			Name:      watcherName.Name,
		},
		WatcherDatabaseName: types.NamespacedName{
			Namespace: watcherName.Namespace,
			Name:      watcher.DatabaseCRName,
		},
		WatcherDatabaseAccount: types.NamespacedName{
			Namespace: watcherName.Namespace,
			Name:      "watcher",
		},
		DatabaseInstance: "openstack",
		//DatabaseHostname: "database-hostname",
		WatcherDatabaseAccountSecret: types.NamespacedName{
			Namespace: watcherName.Namespace,
			Name:      fmt.Sprintf("%s-%s", watcherName.Name, "db-secret"),
		},
		InternalTopLevelSecretName: types.NamespacedName{
			Namespace: watcherName.Namespace,
			Name:      "test-osp-secret",
		},
		PrometheusSecretName: types.NamespacedName{
			Namespace: watcherName.Namespace,
			Name:      "metric-storage-prometheus-endpoint",
		},
		RabbitMqClusterName: "rabbitmq",
		WatcherTransportURL: types.NamespacedName{
			Namespace: watcherName.Namespace,
			Name:      fmt.Sprintf("%s-watcher-transport", watcherName.Name),
		},
		KeystoneServiceName: types.NamespacedName{
			Namespace: watcherName.Namespace,
			Name:      "watcher",
		},
		WatcherAPI: types.NamespacedName{
			Namespace: watcherName.Namespace,
			Name:      "watcher-api",
		},
		WatcherAPIConfigSecret: types.NamespacedName{
			Namespace: watcherName.Namespace,
			Name:      "watcher-api-config-data",
		},
		WatcherDecisionEngine: types.NamespacedName{
			Namespace: watcherName.Namespace,
			Name:      "watcher-decision-engine",
		},
		WatcherDecisionEngineConfigSecret: types.NamespacedName{
			Namespace: watcherName.Namespace,
			Name:      "watcher-decision-engine-config-data",
		},
		WatcherDecisionEngineStatefulSet: types.NamespacedName{
			Namespace: watcherName.Namespace,
			Name:      "watcher-decision-engine",
		},
		WatcherDecisionEngineSecret: types.NamespacedName{
			Namespace: watcherName.Namespace,
			Name:      "watcher-decision-engine-config-data",
		},
		MemcachedNamespace: types.NamespacedName{
			Namespace: watcherName.Namespace,
			Name:      "memcached",
		},
		ServiceAccountName: types.NamespacedName{
			Namespace: watcherName.Namespace,
			Name:      "watcher-" + watcherName.Name,
		},
		RoleName: types.NamespacedName{
			Namespace: watcherName.Namespace,
			Name:      "watcher-" + watcherName.Name + "-role",
		},
		RoleBindingName: types.NamespacedName{
			Namespace: watcherName.Namespace,
			Name:      "watcher-" + watcherName.Name + "-rolebinding",
		},
		WatcherDBSync: types.NamespacedName{
			Namespace: watcherName.Namespace,
			Name:      fmt.Sprintf("%s-db-sync", watcherName.Name),
		},
		WatcherAPIStatefulSet: types.NamespacedName{
			Namespace: watcherName.Namespace,
			Name:      "watcher-api",
		},
		WatcherPublicServiceName: types.NamespacedName{
			Namespace: watcherName.Namespace,
			Name:      "watcher-public",
		},
		WatcherInternalServiceName: types.NamespacedName{
			Namespace: watcherName.Namespace,
			Name:      "watcher-internal",
		},
		WatcherKeystoneEndpointName: types.NamespacedName{
			Namespace: watcherName.Namespace,
			Name:      "watcher",
		},
		WatcherApplier: types.NamespacedName{
			Namespace: watcherName.Namespace,
			Name:      "watcher-applier",
		},
		WatcherApplierConfigSecret: types.NamespacedName{
			Namespace: watcherName.Namespace,
			Name:      "watcher-applier-config-data",
		},
		WatcherApplierStatefulSet: types.NamespacedName{
			Namespace: watcherName.Namespace,
			Name:      "watcher-applier",
		},
		WatcherPublicCertSecret: types.NamespacedName{
			Namespace: watcherName.Namespace,
			Name:      "cert-watcher-public-svc",
		},
		WatcherInternalCertSecret: types.NamespacedName{
			Namespace: watcherName.Namespace,
			Name:      "cert-watcher-internal-svc",
		},
		WatcherApplierSecret: types.NamespacedName{
			Namespace: watcherName.Namespace,
			Name:      "watcher-applier-config-data",
		},
	}
}
