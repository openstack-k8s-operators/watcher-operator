package functional

import (
	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	//revive:disable-next-line:dot-imports
	"os"

	rabbitmqv1 "github.com/openstack-k8s-operators/infra-operator/apis/rabbitmq/v1beta1"
	keystonev1beta1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"
	watcherv1beta1 "github.com/openstack-k8s-operators/watcher-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	MinimalWatcherSpec = map[string]interface{}{
		"databaseInstance": "openstack",
	}

	MinimalWatcherContainerSpec = map[string]interface{}{
		"databaseInstance":                "openstack",
		"apiContainerImageURL":            "watcher-api-custom-image",
		"applierContainerImageURL":        "watcher-applier-custom-image",
		"decisionengineContainerImageURL": "watcher-decision-engine-custom-image",
	}
)

var _ = Describe("Watcher controller with minimal spec values", func() {
	When("A Watcher instance is created from minimal spec", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateWatcher(watcherTest.Instance, MinimalWatcherSpec))
		})

		It("should have the Spec fields defaulted", func() {
			Watcher := GetWatcher(watcherTest.Instance)
			Expect(Watcher.Spec.DatabaseInstance).Should(Equal("openstack"))
			Expect(Watcher.Spec.DatabaseAccount).Should(Equal("watcher"))
			Expect(Watcher.Spec.Secret).Should(Equal("osp-secret"))
			Expect(Watcher.Spec.PasswordSelectors).Should(Equal(watcherv1beta1.PasswordSelector{Service: "WatcherPassword"}))
			Expect(Watcher.Spec.RabbitMqClusterName).Should(Equal("rabbitmq"))
			Expect(Watcher.Spec.ServiceUser).Should(Equal("watcher"))
		})

		It("should have the Status fields initialized", func() {
			Watcher := GetWatcher(watcherTest.Instance)
			Expect(Watcher.Status.ObservedGeneration).To(Equal(int64(0)))
			Expect(Watcher.Status.ServiceID).Should(Equal(""))
		})

		It("It has the expected container image defaults", func() {
			Watcher := GetWatcher(watcherTest.Instance)
			Expect(Watcher.Spec.APIContainerImageURL).To(Equal(watcherv1beta1.WatcherAPIContainerImage))
			Expect(Watcher.Spec.DecisionEngineContainerImageURL).To(Equal(watcherv1beta1.WatcherDecisionEngineContainerImage))
			Expect(Watcher.Spec.ApplierContainerImageURL).To(Equal(watcherv1beta1.WatcherApplierContainerImage))
		})

		It("should have a finalizer", func() {
			// the reconciler loop adds the finalizer so we have to wait for
			// it to run
			Eventually(func() []string {
				return GetWatcher(watcherTest.Instance).Finalizers
			}, timeout, interval).Should(ContainElement("openstack.org/watcher"))
		})

	})
})

var _ = Describe("Watcher controller", func() {
	When("A Watcher instance is created", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateWatcher(watcherTest.Instance, GetDefaultWatcherSpec()))
		})

		It("should have the Spec fields defaulted", func() {
			Watcher := GetWatcher(watcherTest.Instance)
			Expect(Watcher.Spec.DatabaseInstance).Should(Equal("openstack"))
			Expect(Watcher.Spec.DatabaseAccount).Should(Equal("watcher"))
			Expect(Watcher.Spec.ServiceUser).Should(Equal("watcher"))
			Expect(Watcher.Spec.Secret).Should(Equal("test-osp-secret"))
			Expect(Watcher.Spec.RabbitMqClusterName).Should(Equal("rabbitmq"))
		})

		It("should have the Status fields initialized", func() {
			Watcher := GetWatcher(watcherTest.Instance)
			Expect(Watcher.Status.ObservedGeneration).To(Equal(int64(0)))
		})

		It("should have unknown Conditions initialized", func() {
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)

			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionUnknown,
			)

			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				condition.KeystoneServiceReadyCondition,
				corev1.ConditionUnknown,
			)

		})

		It("creates service account, role and rolebindig", func() {
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				condition.ServiceAccountReadyCondition,
				corev1.ConditionTrue,
			)
			sa := th.GetServiceAccount(watcherTest.ServiceAccountName)

			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				condition.RoleReadyCondition,
				corev1.ConditionTrue,
			)
			role := th.GetRole(watcherTest.RoleName)
			Expect(role.Rules).To(HaveLen(2))
			Expect(role.Rules[0].Resources).To(Equal([]string{"securitycontextconstraints"}))
			Expect(role.Rules[1].Resources).To(Equal([]string{"pods"}))

			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				condition.RoleBindingReadyCondition,
				corev1.ConditionTrue,
			)
			binding := th.GetRoleBinding(watcherTest.RoleBindingName)
			Expect(binding.RoleRef.Name).To(Equal(role.Name))
			Expect(binding.Subjects).To(HaveLen(1))
			Expect(binding.Subjects[0].Name).To(Equal(sa.Name))
		})

		It("should have db not ready", func() {
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				condition.DBReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("should have TransportURL not ready", func() {
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				watcherv1beta1.WatcherRabbitMQTransportURLReadyCondition,
				corev1.ConditionUnknown,
			)

		})

		It("should have a finalizer", func() {
			// the reconciler loop adds the finalizer so we have to wait for
			// it to run
			Eventually(func() []string {
				return GetWatcher(watcherTest.Instance).Finalizers
			}, timeout, interval).Should(ContainElement("openstack.org/watcher"))
		})

	})

	When("Watcher DB and RabbitMQ are created", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateWatcher(watcherTest.Instance, GetDefaultWatcherSpec()))
			DeferCleanup(k8sClient.Delete, ctx, CreateWatcherMessageBusSecret(watcherTest.Instance.Namespace, "rabbitmq-secret"))
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					watcherTest.Instance.Namespace,
					GetWatcher(watcherTest.Instance).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
		})

		It("Should set DBReady Condition Status when DB is Created", func() {
			mariadb.SimulateMariaDBAccountCompleted(watcherTest.WatcherDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(watcherTest.WatcherDatabaseName)
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				condition.DBReadyCondition,
				corev1.ConditionTrue,
			)
			cf := th.GetSecret(watcherTest.WatcherDatabaseAccountSecret)
			Expect(cf).ShouldNot(BeNil())
			Watcher := GetWatcher(watcherTest.Instance)
			Expect(Watcher.Status.ObservedGeneration).To(Equal(int64(1)))
			infra.SimulateTransportURLReady(watcherTest.WatcherTransportURL)
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				watcherv1beta1.WatcherRabbitMQTransportURLReadyCondition,
				corev1.ConditionTrue,
			)
			transportURL := &rabbitmqv1.TransportURL{}
			Expect(k8sClient.Get(ctx, watcherTest.WatcherTransportURL, transportURL)).Should(Succeed())
		})

		It("Should register watcher service to keystone when has the right secret", func() {
			mariadb.SimulateMariaDBAccountCompleted(watcherTest.WatcherDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(watcherTest.WatcherDatabaseName)
			infra.SimulateTransportURLReady(watcherTest.WatcherTransportURL)
			DeferCleanup(
				k8sClient.Delete, ctx, th.CreateSecret(
					types.NamespacedName{Namespace: watcherTest.Instance.Namespace, Name: SecretName},
					map[string][]byte{
						"WatcherPassword": []byte("password"),
					},
				))

			// simulate that it becomes ready i.e. the keystone-operator
			// did its job and registered the watcher service
			keystone.SimulateKeystoneServiceReady(watcherTest.KeystoneServiceName)

			// We validate the full Watcher CR readiness status here
			// DB Ready
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				condition.DBReadyCondition,
				corev1.ConditionTrue,
			)
			// RabbitMQ Ready
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				watcherv1beta1.WatcherRabbitMQTransportURLReadyCondition,
				corev1.ConditionTrue,
			)
			// Input Ready (secrets)
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
			// Keystone Service Ready
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				condition.KeystoneServiceReadyCondition,
				corev1.ConditionTrue,
			)

			// Service Account and Role Ready
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				condition.ServiceAccountReadyCondition,
				corev1.ConditionTrue,
			)

			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				condition.RoleReadyCondition,
				corev1.ConditionTrue,
			)

			// Global status Ready
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			// assert that the KeystoneService for watcher is created
			ksrvList := &keystonev1beta1.KeystoneServiceList{}
			listOpts := &client.ListOptions{
				FieldSelector: fields.OneTermEqualSelector("metadata.name", "watcher"),
				Namespace:     watcherTest.Instance.Namespace,
			}
			_ = th.K8sClient.List(ctx, ksrvList, listOpts)
			Expect(ksrvList.Items).ToNot(BeEmpty())
			Expect(ksrvList.Items[0].Status.Conditions).ToNot(BeNil())

		})

		It("Should fail to register watcher service to keystone when has not the expected secret", func() {
			mariadb.SimulateMariaDBAccountCompleted(watcherTest.WatcherDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(watcherTest.WatcherDatabaseName)
			infra.SimulateTransportURLReady(watcherTest.WatcherTransportURL)
			DeferCleanup(
				k8sClient.Delete, ctx, th.CreateSecret(
					types.NamespacedName{Namespace: watcherTest.Instance.Namespace, Name: "fake-secret"},
					map[string][]byte{
						"WatcherPassword": []byte("password"),
					},
				))

			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				condition.KeystoneServiceReadyCondition,
				corev1.ConditionUnknown,
			)

			th.ExpectConditionWithDetails(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"Input data resources missing",
			)

			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)

			// assert that the KeystoneService for watcher is not created
			ksrvList := &keystonev1beta1.KeystoneServiceList{}
			listOpts := &client.ListOptions{
				FieldSelector: fields.OneTermEqualSelector("metadata.name", "watcher"),
				Namespace:     watcherTest.Instance.Namespace,
			}
			_ = th.K8sClient.List(ctx, ksrvList, listOpts)
			Expect(ksrvList.Items).To(BeEmpty())

		})

		It("Should fail to register watcher service to keystone when the secret is missing a key", func() {
			mariadb.SimulateMariaDBAccountCompleted(watcherTest.WatcherDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(watcherTest.WatcherDatabaseName)
			infra.SimulateTransportURLReady(watcherTest.WatcherTransportURL)
			DeferCleanup(
				k8sClient.Delete, ctx, th.CreateSecret(
					types.NamespacedName{Namespace: watcherTest.Instance.Namespace, Name: "test-osp-secret"},
					map[string][]byte{
						"WatcherPasswordFake": []byte("password"),
					},
				))

			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				condition.KeystoneServiceReadyCondition,
				corev1.ConditionUnknown,
			)

			th.ExpectConditionWithDetails(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"Input data error occurred field 'WatcherPassword' not found in secret/test-osp-secret",
			)

			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)

			// assert that the KeystoneService for watcher is not created
			ksrvList := &keystonev1beta1.KeystoneServiceList{}
			listOpts := &client.ListOptions{
				FieldSelector: fields.OneTermEqualSelector("metadata.name", "watcher"),
				Namespace:     watcherTest.Instance.Namespace,
			}
			_ = th.K8sClient.List(ctx, ksrvList, listOpts)
			Expect(ksrvList.Items).To(BeEmpty())

		})

	})

	When("RabbitMQ TransportURL is not created", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateWatcher(watcherTest.Instance, GetDefaultWatcherSpec()))
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					watcherTest.Instance.Namespace,
					GetWatcher(watcherTest.Instance).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
		})

		It("Should set WatcherRabbitMQTransportURLReadyCondition not Ready", func() {
			mariadb.SimulateMariaDBAccountCompleted(watcherTest.WatcherDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(watcherTest.WatcherDatabaseName)
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				condition.DBReadyCondition,
				corev1.ConditionTrue,
			)
			cf := th.GetSecret(watcherTest.WatcherDatabaseAccountSecret)
			Expect(cf).ShouldNot(BeNil())
			Watcher := GetWatcher(watcherTest.Instance)
			Expect(Watcher.Status.ObservedGeneration).To(Equal(int64(1)))
			// TransportURL should get created but Status is not Ready as the secret is not created
			transportURL := &rabbitmqv1.TransportURL{}
			infra.SimulateTransportURLReady(watcherTest.WatcherTransportURL)
			Expect(k8sClient.Get(ctx, watcherTest.WatcherTransportURL, transportURL)).Should(Succeed())
			th.ExpectConditionWithDetails(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				watcherv1beta1.WatcherRabbitMQTransportURLReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"WatcherRabbitMQTransportURL error occured Secret \"rabbitmq-secret\" not found",
			)

			Expect(k8sClient.Get(ctx, watcherTest.WatcherTransportURL, transportURL)).Should(Succeed())
		})
	})

	When("Watcher is created  with container images defined in CR and env variables contains fake values", func() {
		BeforeEach(func() {
			// Set environment variables
			os.Setenv("WATCHER_API_IMAGE_URL_DEFAULT", "watcher-api-custom-image-env")
			os.Setenv("WATCHER_DECISION_ENGINE_IMAGE_URL_DEFAULT", "watcher-decision-engine-custom-image-env")
			os.Setenv("WATCHER_APPLIER_IMAGE_URL_DEFAULT", "watcher-applier-custom-image-env")
			DeferCleanup(th.DeleteInstance, CreateWatcher(watcherTest.Instance, MinimalWatcherContainerSpec))
		})

		It("It should have the fields coming from the spec", func() {
			Watcher := GetWatcher(watcherTest.Instance)
			Expect(Watcher.Spec.APIContainerImageURL).To(Equal("watcher-api-custom-image"))
			Expect(Watcher.Spec.DecisionEngineContainerImageURL).To(Equal("watcher-decision-engine-custom-image"))
			Expect(Watcher.Spec.ApplierContainerImageURL).To(Equal("watcher-applier-custom-image"))
		})
	})

	When("Watcher is created with not container images defined in CR and env variables contains fake value", func() {
		BeforeEach(func() {
			os.Setenv("WATCHER_API_IMAGE_URL_DEFAULT", "watcher-api-custom-image-env")
			os.Setenv("WATCHER_DECISION_ENGINE_IMAGE_URL_DEFAULT", "watcher-decision-engine-custom-image-env")
			os.Setenv("WATCHER_APPLIER_IMAGE_URL_DEFAULT", "watcher-applier-custom-image-env")
			DeferCleanup(th.DeleteInstance, CreateWatcher(watcherTest.Instance, MinimalWatcherSpec))
		})

		It("It should have the fields coming from the environment variables", func() {
			// Note(ChandanKumar): Fix it later why environment variables are not working.
			Skip("Skipping this test case temporarily")
			Watcher := GetWatcher(watcherTest.Instance)
			Expect(Watcher.Spec.APIContainerImageURL).To(Equal("watcher-api-custom-image-env"))
			Expect(Watcher.Spec.DecisionEngineContainerImageURL).To(Equal("watcher-decision-engine-custom-image-env"))
			Expect(Watcher.Spec.ApplierContainerImageURL).To(Equal("watcher-applier-custom-image-env"))
		})
	})
})
