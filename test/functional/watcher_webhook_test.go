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

	rabbitmqv1 "github.com/openstack-k8s-operators/infra-operator/apis/rabbitmq/v1beta1"
	watcherv1 "github.com/openstack-k8s-operators/watcher-operator/api/v1beta1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
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

var _ = Describe("Watcher Webhook Messaging and Notifications", func() {

	Describe("MessagingBus defaulting", func() {
		var spec *watcherv1.WatcherSpecCore

		BeforeEach(func() {
			spec = &watcherv1.WatcherSpecCore{}
		})

		It("should default messagingBus.cluster to rabbitmq when empty", func() {
			spec.Default()

			Expect(spec.MessagingBus.Cluster).To(Equal("rabbitmq"))
			// Note: User and Vhost don't have defaults and remain empty unless explicitly set
			Expect(spec.MessagingBus.User).To(Equal(""))
			Expect(spec.MessagingBus.Vhost).To(Equal(""))
		})

		It("should not override messagingBus.cluster if already set", func() {
			spec.MessagingBus.Cluster = "existing-cluster"
			spec.Default()

			Expect(spec.MessagingBus.Cluster).To(Equal("existing-cluster"))
		})
	})

	Describe("Direct messagingBus field usage", func() {
		var spec *watcherv1.WatcherSpecCore

		It("should preserve messagingBus fields when set directly", func() {
			spec = &watcherv1.WatcherSpecCore{
				RabbitMqClusterName: ptr.To("rabbitmq"),
				MessagingBus: rabbitmqv1.RabbitMqConfig{
					Cluster: "direct-cluster",
					User:    "custom-user",
					Vhost:   "/custom-vhost",
				},
			}
			spec.Default()

			Expect(spec.MessagingBus.Cluster).To(Equal("direct-cluster"))
			Expect(spec.MessagingBus.User).To(Equal("custom-user"))
			Expect(spec.MessagingBus.Vhost).To(Equal("/custom-vhost"))
		})

		It("should use messagingBus.cluster when both old and new fields are set", func() {
			spec = &watcherv1.WatcherSpecCore{
				RabbitMqClusterName: ptr.To("old-rabbitmq"),
				MessagingBus: rabbitmqv1.RabbitMqConfig{
					Cluster: "new-cluster",
				},
			}
			spec.Default()

			// New field should take precedence
			Expect(spec.MessagingBus.Cluster).To(Equal("new-cluster"))
		})
	})

	Describe("NotificationsBus defaulting", func() {
		var spec *watcherv1.WatcherSpecCore

		BeforeEach(func() {
			spec = &watcherv1.WatcherSpecCore{}
		})

		It("should not default notificationsBus.cluster when notificationsBus is present but cluster is empty", func() {
			spec.NotificationsBus = &rabbitmqv1.RabbitMqConfig{}
			spec.Default()

			Expect(spec.NotificationsBus).NotTo(BeNil())
			Expect(spec.NotificationsBus.Cluster).To(Equal(""), "notificationsBus.cluster should not be defaulted - it must be explicitly set")
		})

		It("should not initialize notificationsBus when it is nil", func() {
			spec.Default()

			Expect(spec.NotificationsBus).To(BeNil())
		})

		It("should preserve notificationsBus fields when already set", func() {
			spec.NotificationsBus = &rabbitmqv1.RabbitMqConfig{
				Cluster: "custom-cluster",
				User:    "custom-user",
				Vhost:   "custom-vhost",
			}
			spec.Default()

			Expect(spec.NotificationsBus.Cluster).To(Equal("custom-cluster"))
			Expect(spec.NotificationsBus.User).To(Equal("custom-user"))
			Expect(spec.NotificationsBus.Vhost).To(Equal("custom-vhost"))
		})

		It("should preserve existing notificationsBus.cluster if already set", func() {
			spec.NotificationsBus = &rabbitmqv1.RabbitMqConfig{
				Cluster: "existing-notifications-cluster",
			}
			spec.Default()

			Expect(spec.NotificationsBus.Cluster).To(Equal("existing-notifications-cluster"))
		})
	})

	Describe("NotificationsBus independence from messagingBus", func() {
		var spec *watcherv1.WatcherSpecCore

		It("should keep notificationsBus and messagingBus separate", func() {
			spec = &watcherv1.WatcherSpecCore{
				MessagingBus: rabbitmqv1.RabbitMqConfig{
					Cluster: "rabbitmq-rpc",
					User:    "rpc-user",
					Vhost:   "/rpc-vhost",
				},
				NotificationsBus: &rabbitmqv1.RabbitMqConfig{
					Cluster: "rabbitmq-notifications",
					User:    "notif-user",
					Vhost:   "/notif-vhost",
				},
			}
			spec.Default()

			// Verify MessagingBus fields are preserved
			Expect(spec.MessagingBus.Cluster).To(Equal("rabbitmq-rpc"))
			Expect(spec.MessagingBus.User).To(Equal("rpc-user"))
			Expect(spec.MessagingBus.Vhost).To(Equal("/rpc-vhost"))

			// Verify NotificationsBus fields are preserved and independent
			Expect(spec.NotificationsBus.Cluster).To(Equal("rabbitmq-notifications"))
			Expect(spec.NotificationsBus.User).To(Equal("notif-user"))
			Expect(spec.NotificationsBus.Vhost).To(Equal("/notif-vhost"))
		})
	})

	Describe("Direct notificationsBus field usage", func() {
		var spec *watcherv1.WatcherSpecCore

		It("should preserve notificationsBus fields when set directly without NotificationsBusInstance", func() {
			spec = &watcherv1.WatcherSpecCore{
				RabbitMqClusterName: ptr.To("rabbitmq"),
				NotificationsBus: &rabbitmqv1.RabbitMqConfig{
					Cluster: "direct-notifications-cluster",
					User:    "custom-user",
					Vhost:   "/custom-vhost",
				},
			}
			spec.Default()

			Expect(spec.NotificationsBus.Cluster).To(Equal("direct-notifications-cluster"))
			Expect(spec.NotificationsBus.User).To(Equal("custom-user"))
			Expect(spec.NotificationsBus.Vhost).To(Equal("/custom-vhost"))
		})

		It("should use notificationsBus.cluster when both old and new fields are set", func() {
			spec = &watcherv1.WatcherSpecCore{
				RabbitMqClusterName:      ptr.To("rabbitmq"),
				NotificationsBusInstance: ptr.To("old-notifications"),
				NotificationsBus: &rabbitmqv1.RabbitMqConfig{
					Cluster: "new-notifications-cluster",
				},
			}
			spec.Default()

			// New field should take precedence (already set, so defaulting shouldn't override)
			Expect(spec.NotificationsBus.Cluster).To(Equal("new-notifications-cluster"))
		})
	})

	Describe("Complex scenarios with multiple fields", func() {
		var spec *watcherv1.WatcherSpecCore

		It("should handle messagingBus with only partial fields set", func() {
			spec = &watcherv1.WatcherSpecCore{
				MessagingBus: rabbitmqv1.RabbitMqConfig{
					User:  "messaging-user",
					Vhost: "/messaging-vhost",
				},
			}
			spec.Default()

			// messagingBus.cluster should be defaulted
			Expect(spec.MessagingBus.Cluster).To(Equal("rabbitmq"))
			Expect(spec.MessagingBus.User).To(Equal("messaging-user"))
			Expect(spec.MessagingBus.Vhost).To(Equal("/messaging-vhost"))
		})

		It("should preserve all fields when both buses are fully specified", func() {
			spec = &watcherv1.WatcherSpecCore{
				MessagingBus: rabbitmqv1.RabbitMqConfig{
					Cluster: "custom-rabbitmq",
					User:    "rpc-user",
					Vhost:   "/rpc-vhost",
				},
				NotificationsBus: &rabbitmqv1.RabbitMqConfig{
					Cluster: "custom-notifications",
					User:    "notif-user",
					Vhost:   "/notif-vhost",
				},
			}
			spec.Default()

			Expect(spec.MessagingBus.Cluster).To(Equal("custom-rabbitmq"))
			Expect(spec.MessagingBus.User).To(Equal("rpc-user"))
			Expect(spec.MessagingBus.Vhost).To(Equal("/rpc-vhost"))

			Expect(spec.NotificationsBus.Cluster).To(Equal("custom-notifications"))
			Expect(spec.NotificationsBus.User).To(Equal("notif-user"))
			Expect(spec.NotificationsBus.Vhost).To(Equal("/notif-vhost"))
		})
	})

})

var _ = Describe("Watcher Webhook Update Validation", func() {

	Describe("Validation of deprecated field changes", func() {
		var (
			oldSpec  *watcherv1.WatcherSpecCore
			newSpec  *watcherv1.WatcherSpecCore
			basePath *field.Path
		)

		BeforeEach(func() {
			basePath = field.NewPath("spec")
			oldSpec = &watcherv1.WatcherSpecCore{
				RabbitMqClusterName: ptr.To("rabbitmq"),
				DatabaseInstance:    ptr.To("openstack"),
			}
			// Call Default() to populate messagingBus from rabbitMqClusterName
			oldSpec.Default()

			newSpec = &watcherv1.WatcherSpecCore{
				RabbitMqClusterName: ptr.To("rabbitmq"),
				DatabaseInstance:    ptr.To("openstack"),
			}
			// Call Default() to populate messagingBus from rabbitMqClusterName
			newSpec.Default()
		})

		Describe("RabbitMqClusterName field changes", func() {
			It("should reject changes to RabbitMqClusterName", func() {
				newSpec.RabbitMqClusterName = ptr.To("new-rabbitmq")

				_, errs := newSpec.ValidateUpdate(*oldSpec, basePath, "test-namespace")

				// Expect 2 errors: conflict + forbidden change
				Expect(errs).To(HaveLen(2))

				// Check for both expected errors
				foundForbidden := false
				foundConflict := false
				for _, err := range errs {
					if err.Field == "spec.rabbitMqClusterName" && err.Type == field.ErrorTypeForbidden {
						foundForbidden = true
						Expect(err.Detail).To(ContainSubstring("is deprecated, use"))
						Expect(err.Detail).To(ContainSubstring("messagingBus.cluster"))
					}
					// Conflict error is also on rabbitMqClusterName field (not messagingBus.cluster)
					if err.Field == "spec.rabbitMqClusterName" && err.Type == field.ErrorTypeInvalid {
						foundConflict = true
						Expect(err.Detail).To(ContainSubstring("cannot set both deprecated field"))
						Expect(err.Detail).To(ContainSubstring("messagingBus.cluster"))
					}
				}
				Expect(foundForbidden).To(BeTrue(), "Expected forbidden error for rabbitMqClusterName")
				Expect(foundConflict).To(BeTrue(), "Expected conflict error for rabbitMqClusterName")
			})

			It("should allow update when RabbitMqClusterName remains unchanged", func() {
				// Both specs have the same RabbitMqClusterName
				_, errs := newSpec.ValidateUpdate(*oldSpec, basePath, "test-namespace")

				// Should have no errors related to RabbitMqClusterName
				for _, err := range errs {
					Expect(err.Field).NotTo(Equal("spec.rabbitMqClusterName"))
				}
			})

			It("should have validation error when messagingBus.cluster is empty", func() {
				// Set RabbitMqClusterName to nil and messagingBus.cluster to empty
				oldSpec.RabbitMqClusterName = nil
				oldSpec.MessagingBus.Cluster = ""
				newSpec.RabbitMqClusterName = nil
				newSpec.MessagingBus.Cluster = ""

				_, errs := newSpec.ValidateUpdate(*oldSpec, basePath, "test-namespace")

				// Should have validation error for empty messagingBus.cluster
				found := false
				for _, err := range errs {
					if err.Field == "spec.messagingBus.cluster" {
						found = true
						Expect(err.Type).To(Equal(field.ErrorTypeInvalid))
					}
				}
				Expect(found).To(BeTrue(), "Expected validation error for empty messagingBus.cluster")
			})
		})

		Describe("NotificationsBusInstance field changes", func() {
			BeforeEach(func() {
				oldSpec.NotificationsBusInstance = ptr.To("rabbitmq-notifications")
				oldSpec.Default()

				newSpec.NotificationsBusInstance = ptr.To("rabbitmq-notifications")
				newSpec.Default()
			})

			It("should reject changes to NotificationsBusInstance", func() {
				newSpec.NotificationsBusInstance = ptr.To("new-rabbitmq-notifications")

				_, errs := newSpec.ValidateUpdate(*oldSpec, basePath, "test-namespace")

				// Expect 1 error: forbidden change to deprecated field
				Expect(errs).To(HaveLen(1))

				// Check for forbidden error
				foundForbidden := false
				for _, err := range errs {
					if err.Field == "spec.notificationsBusInstance" && err.Type == field.ErrorTypeForbidden {
						foundForbidden = true
						Expect(err.Detail).To(ContainSubstring("is deprecated, use"))
						Expect(err.Detail).To(ContainSubstring("notificationsBus.cluster"))
					}
				}
				Expect(foundForbidden).To(BeTrue(), "Expected forbidden error for notificationsBusInstance")
			})

			It("should allow update when NotificationsBusInstance remains unchanged", func() {
				// Both specs have the same NotificationsBusInstance
				_, errs := newSpec.ValidateUpdate(*oldSpec, basePath, "test-namespace")

				// Should have no errors related to NotificationsBusInstance
				for _, err := range errs {
					Expect(err.Field).NotTo(Equal("spec.notificationsBusInstance"))
				}
			})

			It("should allow update when NotificationsBusInstance is nil in both specs", func() {
				oldSpec.NotificationsBusInstance = nil
				newSpec.NotificationsBusInstance = nil

				_, errs := newSpec.ValidateUpdate(*oldSpec, basePath, "test-namespace")

				// Should have no errors related to NotificationsBusInstance
				for _, err := range errs {
					Expect(err.Field).NotTo(Equal("spec.notificationsBusInstance"))
				}
			})
		})

		Describe("Multiple deprecated field changes", func() {
			It("should reject changes to both deprecated fields and return multiple errors", func() {
				oldSpec.NotificationsBusInstance = ptr.To("rabbitmq-notifications")
				newSpec.RabbitMqClusterName = ptr.To("new-rabbitmq")
				newSpec.NotificationsBusInstance = ptr.To("new-rabbitmq-notifications")

				_, errs := newSpec.ValidateUpdate(*oldSpec, basePath, "test-namespace")

				// Expect 3 errors: 2 for rabbitMqClusterName (conflict + forbidden) and 1 for notificationsBusInstance (forbidden)
				Expect(errs).To(HaveLen(3))

				// Check for all expected errors
				rabbitMqConflict := false
				rabbitMqForbidden := false
				notificationsForbidden := false
				for _, err := range errs {
					if err.Field == "spec.rabbitMqClusterName" && err.Type == field.ErrorTypeInvalid {
						rabbitMqConflict = true
						Expect(err.Detail).To(ContainSubstring("cannot set both deprecated field"))
					}
					if err.Field == "spec.rabbitMqClusterName" && err.Type == field.ErrorTypeForbidden {
						rabbitMqForbidden = true
						Expect(err.Detail).To(ContainSubstring("is deprecated, use"))
					}
					if err.Field == "spec.notificationsBusInstance" && err.Type == field.ErrorTypeForbidden {
						notificationsForbidden = true
						Expect(err.Detail).To(ContainSubstring("is deprecated, use"))
					}
				}
				Expect(rabbitMqConflict).To(BeTrue(), "Expected conflict error for rabbitMqClusterName")
				Expect(rabbitMqForbidden).To(BeTrue(), "Expected forbidden error for rabbitMqClusterName")
				Expect(notificationsForbidden).To(BeTrue(), "Expected forbidden error for notificationsBusInstance")
			})
		})

		Describe("New messagingBus and notificationsBus field changes", func() {
			It("should allow changes to messagingBus fields", func() {
				oldSpec.MessagingBus = rabbitmqv1.RabbitMqConfig{
					Cluster: "old-cluster",
					User:    "old-user",
					Vhost:   "/old-vhost",
				}
				newSpec.MessagingBus = rabbitmqv1.RabbitMqConfig{
					Cluster: "new-cluster",
					User:    "new-user",
					Vhost:   "/new-vhost",
				}

				_, errs := newSpec.ValidateUpdate(*oldSpec, basePath, "test-namespace")

				// Should have no forbidden errors for messagingBus fields
				for _, err := range errs {
					if err.Type == field.ErrorTypeForbidden {
						Expect(err.Field).NotTo(ContainSubstring("messagingBus"))
					}
				}
			})

			It("should allow changes to notificationsBus fields", func() {
				oldSpec.NotificationsBus = &rabbitmqv1.RabbitMqConfig{
					Cluster: "old-notifications-cluster",
					User:    "old-user",
					Vhost:   "/old-vhost",
				}
				newSpec.NotificationsBus = &rabbitmqv1.RabbitMqConfig{
					Cluster: "new-notifications-cluster",
					User:    "new-user",
					Vhost:   "/new-vhost",
				}

				_, errs := newSpec.ValidateUpdate(*oldSpec, basePath, "test-namespace")

				// Should have no forbidden errors for notificationsBus fields
				for _, err := range errs {
					if err.Type == field.ErrorTypeForbidden {
						Expect(err.Field).NotTo(ContainSubstring("notificationsBus"))
					}
				}
			})
		})
	})

})
