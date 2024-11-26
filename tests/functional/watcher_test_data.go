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
	DatabaseInstance             string
	RabbitMqClusterName          string
	Instance                     types.NamespacedName
	Watcher                      types.NamespacedName
	WatcherDatabaseName          types.NamespacedName
	WatcherDatabaseAccount       types.NamespacedName
	WatcherDatabaseAccountSecret types.NamespacedName
	InternalTopLevelSecretName   types.NamespacedName
	WatcherTransportURL          types.NamespacedName
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
		RabbitMqClusterName: "rabbitmq",
		WatcherTransportURL: types.NamespacedName{
			Namespace: watcherName.Namespace,
			Name:      fmt.Sprintf("%s-watcher-transport", watcherName.Name),
		},
	}
}
