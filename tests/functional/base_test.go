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
	. "github.com/onsi/gomega" //revive:disable:dot-imports

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	watcherv1 "github.com/openstack-k8s-operators/watcher-operator/api/v1beta1"
)

func GetDefaultWatcherSpec() map[string]interface{} {
	return map[string]interface{}{
		"databaseInstance": "openstack",
		"secret":           SecretName,
	}
}

func GetDefaultWatcherAPISpec() map[string]interface{} {
	return map[string]interface{}{
		"databaseInstance": "openstack",
		"secret":           SecretName,
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
