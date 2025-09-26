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
package functional

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/types"
)

const SamplesDir = "../../config/samples/"

func ReadSample(sampleFileName string) map[string]any {
	rawSample := make(map[string]any)

	bytes, err := os.ReadFile(filepath.Join(SamplesDir, sampleFileName)) //nolint:gosec // G304: File path is sanitized and controlled in test environment
	Expect(err).ShouldNot(HaveOccurred())
	Expect(yaml.Unmarshal(bytes, rawSample)).Should(Succeed())

	return rawSample
}

// TODO(amoralej) This is creating the test to validate Watcher sample.
// For the rest of CRs we will add additional tests when creating the basic structure
// and envtest.

func CreateWatcherFromSample(sampleFileName string, name types.NamespacedName) types.NamespacedName {
	raw := ReadSample(sampleFileName)
	instance := CreateWatcher(name, raw["spec"].(map[string]any))
	DeferCleanup(th.DeleteInstance, instance)
	return types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
}

func CreateWatcherAPIFromSample(sampleFileName string, name types.NamespacedName) types.NamespacedName {
	raw := ReadSample(sampleFileName)
	instance := CreateWatcherAPI(name, raw["spec"].(map[string]any))
	DeferCleanup(th.DeleteInstance, instance)
	return types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
}

func CreateWatcherApplierFromSample(sampleFileName string, name types.NamespacedName) types.NamespacedName {
	raw := ReadSample(sampleFileName)
	instance := CreateWatcherApplier(name, raw["spec"].(map[string]any))
	DeferCleanup(th.DeleteInstance, instance)
	return types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
}

func CreateWatcherDecisionEngineFromSample(sampleFileName string, name types.NamespacedName) types.NamespacedName {
	raw := ReadSample(sampleFileName)
	instance := CreateWatcherDecisionEngine(name, raw["spec"].(map[string]any))
	DeferCleanup(th.DeleteInstance, instance)
	return types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
}

// This is a set of test for our samples. It only validates that the sample
// file has all the required field with proper types. But it does not
// validate that using a sample file will result in a working deployment.
// TODO(gibi): By building up all the prerequisites (e.g. MariaDBDatabase) in
// the test and by simulating Job and Deployment success we could assert
// that each sample creates a CR in Ready state.
var _ = Describe("Samples", func() {

	When("watcher_v1beta1_watcher.yaml sample is applied", func() {
		It("Watcher is created", func() {
			name := CreateWatcherFromSample("watcher_v1beta1_watcher.yaml", watcherTest.Instance)
			GetWatcher(name)
		})
	})

	When("watcher_v1beta1_watcherapi.yaml sample is applied", func() {
		It("WatcherAPI is created", func() {
			name := CreateWatcherAPIFromSample("watcher_v1beta1_watcherapi.yaml", watcherTest.WatcherAPI)
			GetWatcherAPI(name)
		})
	})

	When("watcher_v1beta1_watcherapplier.yaml sample is applied", func() {
		It("WatcherApplier is created", func() {
			name := CreateWatcherApplierFromSample("watcher_v1beta1_watcherapplier.yaml", watcherTest.WatcherApplier)
			GetWatcherApplier(name)
		})
	})

	When("watcher_v1beta1_watcherdecisionengine.yaml sample is applied", func() {
		It("WatcherDecisionEngine is created", func() {
			name := CreateWatcherDecisionEngineFromSample("watcher_v1beta1_watcherdecisionengine.yaml", watcherTest.WatcherDecisionEngine)
			GetWatcherDecisionEngine(name)
		})
	})

})
