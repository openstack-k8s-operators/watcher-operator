/*
Copyright 2025.

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
	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	"k8s.io/utils/ptr"

	watcherv1 "github.com/openstack-k8s-operators/watcher-operator/api/v1beta1"
)

var _ = Describe("SetDefaultRouteAnnotations", func() {
	const (
		haProxyAnno = "haproxy.router.openshift.io/timeout"
		watcherAnno = "api.watcher.openstack.org/timeout"
	)

	var (
		spec        *watcherv1.WatcherSpecCore
		annotations *map[string]string
	)

	BeforeEach(func() {
		// Set up a WatcherSpecCore
		spec = &watcherv1.WatcherSpecCore{}
		// Start with empty annotations for each test
		annotations = ptr.To(make(map[string]string))
	})

	When("annotations map is empty", func() {

		It("should set both HAProxy and Watcher annotations with APITimeout value", func() {
			spec.SetDefaultRouteAnnotations(*annotations)

			Expect(*annotations).To(HaveKeyWithValue(haProxyAnno, "60s"))
			Expect(*annotations).To(HaveKeyWithValue(watcherAnno, "60s"))
		})
	})

	When("neither annotation exists but other annotations are present", func() {
		BeforeEach(func() {
			(*annotations)["some.other.annotation"] = "value"
		})

		It("should set both HAProxy and Watcher annotations without affecting other annotations", func() {
			spec.SetDefaultRouteAnnotations(*annotations)

			Expect(*annotations).To(HaveKeyWithValue(haProxyAnno, "60s"))
			Expect(*annotations).To(HaveKeyWithValue(watcherAnno, "60s"))
			Expect(*annotations).To(HaveKeyWithValue("some.other.annotation", "value"))
		})
	})

	When("only HAProxy annotation exists (manually set by human operator)", func() {
		BeforeEach(func() {
			(*annotations)[haProxyAnno] = "120s"
		})

		It("should not modify any annotations (respects manual configuration)", func() {
			spec.SetDefaultRouteAnnotations(*annotations)

			Expect(*annotations).To(HaveKeyWithValue(haProxyAnno, "120s"))
			Expect(*annotations).NotTo(HaveKey(watcherAnno))
		})
	})

	When("only Watcher annotation exists", func() {
		BeforeEach(func() {
			(*annotations)[watcherAnno] = "30s"
		})

		It("should set HAProxy annotation to match current APITimeout", func() {
			spec.SetDefaultRouteAnnotations(*annotations)

			Expect(*annotations).To(HaveKeyWithValue(haProxyAnno, "60s"))
			Expect(*annotations).To(HaveKeyWithValue(watcherAnno, "60s"))
		})
	})

	When("both annotations exist and match but are different to APITimeout", func() {
		BeforeEach(func() {
			(*annotations)[haProxyAnno] = "30s"
			(*annotations)[watcherAnno] = "30s"
		})

		It("should update both annotations to current APITimeout value", func() {
			spec.SetDefaultRouteAnnotations(*annotations)

			Expect(*annotations).To(HaveKeyWithValue(haProxyAnno, "60s"))
			Expect(*annotations).To(HaveKeyWithValue(watcherAnno, "60s"))
		})
	})

	When("both annotations exist but don't match (human modified HAProxy manually)", func() {
		BeforeEach(func() {
			(*annotations)[haProxyAnno] = "180s" // Human set this manually
			(*annotations)[watcherAnno] = "60s"  // Operator had set this before
		})

		It("should remove only the Watcher annotation (preserves manual HAProxy setting)", func() {
			spec.SetDefaultRouteAnnotations(*annotations)

			Expect(*annotations).To(HaveKeyWithValue(haProxyAnno, "180s"))
			Expect(*annotations).NotTo(HaveKey(watcherAnno))
		})
	})

	When("when APITimeout has different values", func() {

		It("should set annotations with 30s timeout", func() {
			spec.APITimeout = ptr.To(30)
			spec.SetDefaultRouteAnnotations(*annotations)

			Expect(*annotations).To(HaveKeyWithValue(haProxyAnno, "30s"))
			Expect(*annotations).To(HaveKeyWithValue(watcherAnno, "30s"))
		})

		It("should set annotations with 300s timeout", func() {
			spec.APITimeout = ptr.To(300)
			spec.SetDefaultRouteAnnotations(*annotations)

			Expect(*annotations).To(HaveKeyWithValue(haProxyAnno, "300s"))
			Expect(*annotations).To(HaveKeyWithValue(watcherAnno, "300s"))
		})
	})

})
