package functional

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	//revive:disable-next-line:dot-imports
	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
	watcherv1beta1 "github.com/openstack-k8s-operators/watcher-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
)

var (
	MinimalWatcherApplierSpec = map[string]interface{}{
		"secret":            "osp-secret",
		"memcachedInstance": "memcached",
	}
)

var _ = Describe("WatcherApplier controller with minimal spec values", func() {
	When("A WatcherApplier instance is created from minimal spec", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateWatcherApplier(watcherTest.WatcherApplier, MinimalWatcherApplierSpec))
		})

		It("should have the Spec fields defaulted", func() {
			WatcherApplier := GetWatcherApplier(watcherTest.WatcherApplier)
			Expect(WatcherApplier.Spec.Secret).Should(Equal("osp-secret"))
			Expect(*(WatcherApplier.Spec.MemcachedInstance)).Should(Equal("memcached"))
			Expect(WatcherApplier.Spec.PasswordSelectors).Should(Equal(watcherv1beta1.PasswordSelector{Service: ptr.To("WatcherPassword")}))
		})

		It("should have the Status fields initialized", func() {
			WatcherApplier := GetWatcherApplier(watcherTest.WatcherApplier)
			Expect(WatcherApplier.Status.ObservedGeneration).To(Equal(int64(0)))
		})

		It("should have a finalizer", func() {
			// the reconciler loop adds the finalizer so we have to wait for
			// it to run
			Eventually(func() []string {
				return GetWatcherApplier(watcherTest.WatcherApplier).Finalizers
			}, timeout, interval).Should(ContainElement("openstack.org/watcherapplier"))
		})

	})
})

var _ = Describe("WatcherApplier controller", func() {
	When("A WatcherApplier instance is created", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateWatcherApplier(watcherTest.WatcherApplier, GetDefaultWatcherApplierSpec()))
		})

		It("should have the Spec fields defaulted", func() {
			WatcherApplier := GetWatcherApplier(watcherTest.WatcherApplier)
			Expect(WatcherApplier.Spec.Secret).Should(Equal("test-osp-secret"))
			Expect(*(WatcherApplier.Spec.MemcachedInstance)).Should(Equal("memcached"))
			Expect(WatcherApplier.Spec.PasswordSelectors).Should(Equal(watcherv1beta1.PasswordSelector{Service: ptr.To("WatcherPassword")}))
		})

		It("should have the Status fields initialized", func() {
			WatcherApplier := GetWatcherApplier(watcherTest.WatcherApplier)
			Expect(WatcherApplier.Status.ObservedGeneration).To(Equal(int64(0)))
		})

		It("should have ReadyCondition false", func() {
			th.ExpectCondition(
				watcherTest.WatcherApplier,
				ConditionGetterFunc(WatcherApplierConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("should have input not ready", func() {
			th.ExpectCondition(
				watcherTest.WatcherApplier,
				ConditionGetterFunc(WatcherApplierConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("should have service config input unknown", func() {
			th.ExpectCondition(
				watcherTest.WatcherApplier,
				ConditionGetterFunc(WatcherApplierConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionUnknown,
			)
		})

		It("should have a finalizer", func() {
			// the reconciler loop adds the finalizer so we have to wait for
			// it to run
			Eventually(func() []string {
				return GetWatcherApplier(watcherTest.WatcherApplier).Finalizers
			}, timeout, interval).Should(ContainElement("openstack.org/watcherapplier"))
		})

		It("should return false when using isReady method", func() {
			WatcherApplier := GetWatcherApplier(watcherTest.WatcherApplier)
			Expect(WatcherApplier.IsReady()).Should(BeFalse())
		})
	})
	When("the secret is created with all the expected fields and has all the required infra", func() {
		var keystoneAPIName types.NamespacedName

		BeforeEach(func() {
			secret := th.CreateSecret(
				watcherTest.InternalTopLevelSecretName,
				map[string][]byte{
					"WatcherPassword":       []byte("service-password"),
					"transport_url":         []byte("url"),
					"database_username":     []byte("username"),
					"database_password":     []byte("password"),
					"database_hostname":     []byte("hostname"),
					"database_account":      []byte("watcher"),
					"01-global-custom.conf": []byte(""),
					"notification_url":      []byte(""),
				},
			)
			DeferCleanup(k8sClient.Delete, ctx, secret)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					watcherTest.WatcherApplier.Namespace,
					"openstack",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			mariadb.CreateMariaDBAccountAndSecret(
				watcherTest.WatcherDatabaseAccount,
				mariadbv1.MariaDBAccountSpec{
					UserName: "watcher",
				},
			)
			mariadb.CreateMariaDBDatabase(
				watcherTest.WatcherApplier.Namespace,
				"watcher",
				mariadbv1.MariaDBDatabaseSpec{
					Name: "watcher",
				},
			)
			mariadb.SimulateMariaDBAccountCompleted(watcherTest.WatcherDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(watcherTest.WatcherDatabaseName)
			keystoneAPIName = keystone.CreateKeystoneAPI(watcherTest.WatcherApplier.Namespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPIName)
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.WatcherApplier.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
			DeferCleanup(th.DeleteInstance, CreateWatcherApplier(watcherTest.WatcherApplier, GetDefaultWatcherApplierSpec()))
		})
		It("should have input ready", func() {
			th.ExpectCondition(
				watcherTest.WatcherApplier,
				ConditionGetterFunc(WatcherApplierConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("should have memcached ready true", func() {
			th.ExpectCondition(
				watcherTest.WatcherApplier,
				ConditionGetterFunc(WatcherApplierConditionGetter),
				condition.MemcachedReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("should have config service input ready", func() {
			th.ExpectCondition(
				watcherTest.WatcherApplier,
				ConditionGetterFunc(WatcherApplierConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("should have cretaed the config secrete with the expected content", func() {
			th.ExpectCondition(
				watcherTest.WatcherApplier,
				ConditionGetterFunc(WatcherApplierConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
			// assert that the top level secret is created with proper content
			createdSecret := th.GetSecret(watcherTest.WatcherApplierSecret)
			Expect(createdSecret).ShouldNot(BeNil())
			Expect(createdSecret.Data["00-default.conf"]).ShouldNot(BeNil())

			// extract default config data
			configData := createdSecret.Data["00-default.conf"]
			Expect(configData).ShouldNot(BeNil())

			// indentaion is forced by use of raw literal
			expectedSections := []string{`
[cinder_client]
endpoint_type = internal`, `
[glance_client]
endpoint_type = internal`, `
[ironic_client]
endpoint_type = internal`, `
[keystone_client]
interface = internal`, `
[neutron_client]
endpoint_type = internal`, `
[nova_client]
endpoint_type = internal`, `
[placement_client]
interface = internal`, `
[oslo_messaging_notifications]

driver = noop`,
			}
			for _, val := range expectedSections {
				Expect(string(configData)).Should(ContainSubstring(val))
			}
			unexpectedNotificationSection := `
[oslo_messaging_notifications]

driver = messagingv2
transport_url =`
			Expect(string(configData)).Should(Not(ContainSubstring(unexpectedNotificationSection)))
		})
		It("creates a deployment for the watcher-applier service", func() {
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherApplierStatefulSet)
			th.ExpectCondition(
				watcherTest.WatcherApplier,
				ConditionGetterFunc(WatcherApplierConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)

			statefulset := th.GetStatefulSet(watcherTest.WatcherApplierStatefulSet)
			Expect(statefulset.Spec.Template.Spec.ServiceAccountName).To(Equal("watcher-sa"))
			Expect(int(*statefulset.Spec.Replicas)).To(Equal(1))
			Expect(statefulset.Spec.Template.Spec.Volumes).To(HaveLen(3))
			Expect(statefulset.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(statefulset.Spec.Selector.MatchLabels).To(Equal(map[string]string{"service": "watcher-applier"}))

			container := statefulset.Spec.Template.Spec.Containers[0]
			Expect(container.VolumeMounts).To(HaveLen(4))
			Expect(container.Image).To(Equal("test://watcher"))

			probeCmd := []string{
				"/usr/bin/pgrep", "-r", "DRST", "watcher-applier",
			}
			Expect(container.StartupProbe.Exec.Command).To(Equal(probeCmd))
			Expect(container.LivenessProbe.Exec.Command).To(Equal(probeCmd))
			Expect(container.ReadinessProbe.Exec.Command).To(Equal(probeCmd))
		})
		It("should return true when using isReady method", func() {
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherApplierStatefulSet)

			th.ExpectCondition(
				watcherTest.WatcherApplier,
				ConditionGetterFunc(WatcherApplierConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			WatcherApplier := GetWatcherApplier(watcherTest.WatcherApplier)
			Expect(WatcherApplier.IsReady()).Should(BeTrue())
		})
		It("updates the KeystoneAuthURL if keystone internal endpoint changes", func() {
			newInternalEndpoint := "https://keystone-internal"

			keystone.UpdateKeystoneAPIEndpoint(keystoneAPIName, "internal", newInternalEndpoint)
			logger.Info("Reconfigured")

			th.ExpectCondition(
				watcherTest.WatcherApplier,
				ConditionGetterFunc(WatcherApplierConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				confSecret := th.GetSecret(watcherTest.WatcherApplierConfigSecret)
				g.Expect(confSecret).ShouldNot(BeNil())

				conf := string(confSecret.Data["00-default.conf"])
				g.Expect(conf).Should(
					ContainSubstring("auth_url = %s", newInternalEndpoint))
			}, timeout, interval).Should(Succeed())
		})
	})
	When("the secret is created but missing fields", func() {
		BeforeEach(func() {
			secret := th.CreateSecret(
				watcherTest.InternalTopLevelSecretName,
				map[string][]byte{},
			)
			DeferCleanup(k8sClient.Delete, ctx, secret)
			DeferCleanup(th.DeleteInstance, CreateWatcherApplier(watcherTest.WatcherApplier, GetDefaultWatcherApplierSpec()))
		})
		It("should have input false", func() {
			errorString := fmt.Sprintf(
				condition.InputReadyErrorMessage,
				"field 'WatcherPassword' not found in secret/test-osp-secret",
			)
			th.ExpectConditionWithDetails(
				watcherTest.WatcherApplier,
				ConditionGetterFunc(WatcherApplierConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				errorString,
			)
		})
		It("should have config service input unknown", func() {
			th.ExpectCondition(
				watcherTest.WatcherApplier,
				ConditionGetterFunc(WatcherApplierConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionUnknown,
			)
		})
	})
	When("A WatcherApplier instance without secret is created", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateWatcherApplier(watcherTest.WatcherApplier, GetDefaultWatcherApplierSpec()))
		})
		It("is missing the secret", func() {
			th.ExpectConditionWithDetails(
				watcherTest.WatcherApplier,
				ConditionGetterFunc(WatcherApplierConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				condition.InputReadyWaitingMessage,
			)
		})
	})
	When("secret and db are created, but there is no memcached", func() {
		BeforeEach(func() {
			secret := th.CreateSecret(
				watcherTest.InternalTopLevelSecretName,
				map[string][]byte{
					"WatcherPassword":       []byte("service-password"),
					"transport_url":         []byte("url"),
					"database_username":     []byte("username"),
					"database_password":     []byte("password"),
					"database_hostname":     []byte("hostname"),
					"database_account":      []byte("watcher"),
					"01-global-custom.conf": []byte(""),
					"notification_url":      []byte(""),
				},
			)
			DeferCleanup(k8sClient.Delete, ctx, secret)

			DeferCleanup(th.DeleteInstance, CreateWatcherApplier(watcherTest.WatcherApplier, GetDefaultWatcherApplierSpec()))
		})
		It("should have input ready true", func() {
			th.ExpectCondition(
				watcherTest.WatcherApplier,
				ConditionGetterFunc(WatcherApplierConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("should have memcached ready false", func() {
			th.ExpectConditionWithDetails(
				watcherTest.WatcherApplier,
				ConditionGetterFunc(WatcherApplierConditionGetter),
				condition.MemcachedReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				condition.MemcachedReadyWaitingMessage,
			)
		})
	})
	When("secret, db and memcached are created, but there is no keystoneapi", func() {
		BeforeEach(func() {
			secret := th.CreateSecret(
				watcherTest.InternalTopLevelSecretName,
				map[string][]byte{
					"WatcherPassword":       []byte("service-password"),
					"transport_url":         []byte("url"),
					"database_username":     []byte("username"),
					"database_password":     []byte("password"),
					"database_hostname":     []byte("hostname"),
					"database_account":      []byte("watcher"),
					"01-global-custom.conf": []byte(""),
					"notification_url":      []byte(""),
				},
			)
			DeferCleanup(k8sClient.Delete, ctx, secret)
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.WatcherApplier.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
			DeferCleanup(th.DeleteInstance, CreateWatcherApplier(watcherTest.WatcherApplier, GetDefaultWatcherApplierSpec()))

		})
		It("should have input ready true", func() {
			th.ExpectCondition(
				watcherTest.WatcherApplier,
				ConditionGetterFunc(WatcherApplierConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("should have memcached ready true", func() {
			th.ExpectCondition(
				watcherTest.WatcherApplier,
				ConditionGetterFunc(WatcherApplierConditionGetter),
				condition.MemcachedReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("should have config service input unknown", func() {
			errorString := fmt.Sprintf(
				condition.ServiceConfigReadyErrorMessage,
				"keystoneAPI not found",
			)
			th.ExpectConditionWithDetails(
				watcherTest.WatcherApplier,
				ConditionGetterFunc(WatcherApplierConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				errorString,
			)
		})
	})
	When("WatcherApplier is created with a wrong topologyRef", func() {
		BeforeEach(func() {
			secret := th.CreateSecret(
				watcherTest.InternalTopLevelSecretName,
				map[string][]byte{
					"WatcherPassword":       []byte("service-password"),
					"transport_url":         []byte("url"),
					"database_username":     []byte("username"),
					"database_password":     []byte("password"),
					"database_hostname":     []byte("hostname"),
					"database_account":      []byte("watcher"),
					"01-global-custom.conf": []byte(""),
					"notification_url":      []byte(""),
				},
			)
			DeferCleanup(k8sClient.Delete, ctx, secret)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					watcherTest.WatcherApplier.Namespace,
					"openstack",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			mariadb.CreateMariaDBAccountAndSecret(
				watcherTest.WatcherDatabaseAccount,
				mariadbv1.MariaDBAccountSpec{
					UserName: "watcher",
				},
			)
			mariadb.CreateMariaDBDatabase(
				watcherTest.WatcherApplier.Namespace,
				"watcher",
				mariadbv1.MariaDBDatabaseSpec{
					Name: "watcher",
				},
			)
			mariadb.SimulateMariaDBAccountCompleted(watcherTest.WatcherDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(watcherTest.WatcherDatabaseName)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(watcherTest.WatcherApplier.Namespace))
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.WatcherApplier.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
			spec := GetDefaultWatcherApplierSpec()
			spec["topologyRef"] = map[string]interface{}{"name": "foo"}
			DeferCleanup(th.DeleteInstance, CreateWatcherApplier(watcherTest.WatcherApplier, spec))
		})
		It("points to a non existing topology CR", func() {
			th.ExpectCondition(
				watcherTest.WatcherApplier,
				ConditionGetterFunc(WatcherApplierConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				watcherTest.WatcherApplier,
				ConditionGetterFunc(WatcherApplierConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})
	})

	When("WatcherApplier is created with topology", func() {
		var topologyRefApplier topologyv1.TopoRef
		var topologyRefAlt topologyv1.TopoRef
		var expectedTopologySpec []corev1.TopologySpreadConstraint
		BeforeEach(func() {
			var topologySpec map[string]interface{}
			// Build the topology Spec
			topologySpec, expectedTopologySpec = GetSampleTopologySpec("watcher-applier")
			_ = expectedTopologySpec
			// Create Test Topologies
			_, topologyRefApplier = infra.CreateTopology(
				types.NamespacedName{
					Namespace: namespace,
					Name:      "watcherapplier"},
				topologySpec)
			_, topologyRefAlt = infra.CreateTopology(
				types.NamespacedName{
					Namespace: namespace,
					Name:      "watcher"},
				topologySpec)

			secret := th.CreateSecret(
				watcherTest.InternalTopLevelSecretName,
				map[string][]byte{
					"WatcherPassword":       []byte("service-password"),
					"transport_url":         []byte("url"),
					"database_username":     []byte("username"),
					"database_password":     []byte("password"),
					"database_hostname":     []byte("hostname"),
					"database_account":      []byte("watcher"),
					"01-global-custom.conf": []byte(""),
					"notification_url":      []byte(""),
				},
			)
			DeferCleanup(k8sClient.Delete, ctx, secret)
			spec := GetDefaultWatcherApplierSpec()
			spec["topologyRef"] = map[string]interface{}{"name": topologyRefApplier.Name}
			DeferCleanup(th.DeleteInstance, CreateWatcherApplier(watcherTest.WatcherApplier, spec))
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(watcherTest.WatcherApplier.Namespace))
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.WatcherApplier.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					watcherTest.WatcherApplier.Namespace,
					"openstack",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			mariadb.CreateMariaDBAccountAndSecret(
				watcherTest.WatcherDatabaseAccount,
				mariadbv1.MariaDBAccountSpec{
					UserName: "watcher",
				},
			)
			mariadb.CreateMariaDBDatabase(
				watcherTest.WatcherApplier.Namespace,
				"watcher",
				mariadbv1.MariaDBDatabaseSpec{
					Name: "watcher",
				},
			)
			mariadb.SimulateMariaDBAccountCompleted(watcherTest.WatcherDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(watcherTest.WatcherDatabaseName)
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherApplierStatefulSet)

		})
		It("sets lastAppliedTopology field in WatcherApplier topology .Status", func() {

			WatcherApplier := GetWatcherApplier(watcherTest.WatcherApplier)

			Expect(WatcherApplier.Status.LastAppliedTopology).ToNot(BeNil())
			Expect(WatcherApplier.Status.LastAppliedTopology).To(Equal(&topologyRefApplier))

			th.ExpectCondition(
				watcherTest.WatcherApplier,
				ConditionGetterFunc(WatcherApplierConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				ss := th.GetStatefulSet(watcherTest.WatcherApplierStatefulSet)
				podTemplate := ss.Spec.Template.Spec
				g.Expect(podTemplate.TopologySpreadConstraints).ToNot(BeNil())
				// No default Pod Antiaffinity is applied
				g.Expect(podTemplate.Affinity).To(BeNil())
			}, timeout, interval).Should(Succeed())

			// Check finalizer
			Eventually(func(g Gomega) {
				tp := infra.GetTopology(types.NamespacedName{
					Name:      topologyRefApplier.Name,
					Namespace: topologyRefApplier.Namespace,
				})
				finalizers := tp.GetFinalizers()
				g.Expect(finalizers).To(ContainElement(
					fmt.Sprintf("openstack.org/watcherapplier-%s", watcherTest.WatcherApplier.Name)))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				tpAlt := infra.GetTopology(types.NamespacedName{
					Name:      topologyRefAlt.Name,
					Namespace: topologyRefAlt.Namespace,
				})
				finalizers := tpAlt.GetFinalizers()
				g.Expect(finalizers).ToNot(ContainElement(
					fmt.Sprintf("openstack.org/watcherapplier-%s", watcherTest.WatcherApplier.Name)))
			}, timeout, interval).Should(Succeed())
		})
		It("updates lastAppliedTopology in WatcherApplier .Status", func() {
			Eventually(func(g Gomega) {
				WatcherApplier := GetWatcherApplier(watcherTest.WatcherApplier)
				WatcherApplier.Spec.TopologyRef.Name = topologyRefAlt.Name
				g.Expect(k8sClient.Update(ctx, WatcherApplier)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				WatcherApplier := GetWatcherApplier(watcherTest.WatcherApplier)
				g.Expect(WatcherApplier.Status.LastAppliedTopology).ToNot(BeNil())
				g.Expect(WatcherApplier.Status.LastAppliedTopology).To(Equal(&topologyRefAlt))
			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				watcherTest.WatcherApplier,
				ConditionGetterFunc(WatcherApplierConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				ss := th.GetStatefulSet(watcherTest.WatcherApplier)
				podTemplate := ss.Spec.Template.Spec
				g.Expect(podTemplate.TopologySpreadConstraints).ToNot(BeNil())
				// No default Pod Antiaffinity is applied
				g.Expect(podTemplate.Affinity).To(BeNil())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				ss := th.GetStatefulSet(watcherTest.WatcherApplier)
				podTemplate := ss.Spec.Template.Spec
				g.Expect(podTemplate.TopologySpreadConstraints).To(Equal(expectedTopologySpec))
			}, timeout, interval).Should(Succeed())

			// Check finalizer is set to topologyRefAlt and is not set to
			// topologyRef
			Eventually(func(g Gomega) {
				tp := infra.GetTopology(types.NamespacedName{
					Name:      topologyRefApplier.Name,
					Namespace: topologyRefApplier.Namespace,
				})
				finalizers := tp.GetFinalizers()
				g.Expect(finalizers).ToNot(ContainElement(
					fmt.Sprintf("openstack.org/watcherapplier-%s", watcherTest.WatcherApplier.Name)))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				tpAlt := infra.GetTopology(types.NamespacedName{
					Name:      topologyRefAlt.Name,
					Namespace: topologyRefAlt.Namespace,
				})
				finalizers := tpAlt.GetFinalizers()
				g.Expect(finalizers).To(ContainElement(
					fmt.Sprintf("openstack.org/watcherapplier-%s", watcherTest.WatcherApplier.Name)))
			}, timeout, interval).Should(Succeed())
		})
		It("removes topologyRef from WatcherApplier spec", func() {
			Eventually(func(g Gomega) {
				WatcherApplier := GetWatcherApplier(watcherTest.WatcherApplier)
				WatcherApplier.Spec.TopologyRef = nil
				g.Expect(k8sClient.Update(ctx, WatcherApplier)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				WatcherApplier := GetWatcherApplier(watcherTest.WatcherApplier)
				g.Expect(WatcherApplier.Status.LastAppliedTopology).Should(BeNil())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				ss := th.GetStatefulSet(watcherTest.WatcherApplier)
				podTemplate := ss.Spec.Template.Spec
				g.Expect(podTemplate.TopologySpreadConstraints).To(BeNil())
				// Default Pod AntiAffinity is applied
				g.Expect(podTemplate.Affinity).ToNot(BeNil())
			}, timeout, interval).Should(Succeed())

			// Check finalizer is not present anymore
			Eventually(func(g Gomega) {
				tp := infra.GetTopology(types.NamespacedName{
					Name:      topologyRefApplier.Name,
					Namespace: topologyRefApplier.Namespace,
				})
				finalizers := tp.GetFinalizers()
				g.Expect(finalizers).ToNot(ContainElement(
					fmt.Sprintf("openstack.org/watcherapplier-%s", watcherTest.WatcherApplier.Name)))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				tpAlt := infra.GetTopology(types.NamespacedName{
					Name:      topologyRefAlt.Name,
					Namespace: topologyRefAlt.Namespace,
				})
				finalizers := tpAlt.GetFinalizers()
				g.Expect(finalizers).ToNot(ContainElement(
					fmt.Sprintf("openstack.org/watcherapplier-%s", watcherTest.WatcherApplier.Name)))
			}, timeout, interval).Should(Succeed())
		})
	})
	When("the secret is created with a notification_url field in the secret", func() {
		var keystoneAPIName types.NamespacedName

		BeforeEach(func() {
			secret := th.CreateSecret(
				watcherTest.InternalTopLevelSecretName,
				map[string][]byte{
					"WatcherPassword":       []byte("service-password"),
					"transport_url":         []byte("url"),
					"database_username":     []byte("username"),
					"database_password":     []byte("password"),
					"database_hostname":     []byte("hostname"),
					"database_account":      []byte("watcher"),
					"01-global-custom.conf": []byte(""),
					"notification_url":      []byte("rabbit://rabbitmq-notification-secret/fake"),
				},
			)
			DeferCleanup(k8sClient.Delete, ctx, secret)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					watcherTest.WatcherApplier.Namespace,
					"openstack",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			mariadb.CreateMariaDBAccountAndSecret(
				watcherTest.WatcherDatabaseAccount,
				mariadbv1.MariaDBAccountSpec{
					UserName: "watcher",
				},
			)
			mariadb.CreateMariaDBDatabase(
				watcherTest.WatcherApplier.Namespace,
				"watcher",
				mariadbv1.MariaDBDatabaseSpec{
					Name: "watcher",
				},
			)
			mariadb.SimulateMariaDBAccountCompleted(watcherTest.WatcherDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(watcherTest.WatcherDatabaseName)
			keystoneAPIName = keystone.CreateKeystoneAPI(watcherTest.WatcherApplier.Namespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPIName)
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.WatcherApplier.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
			DeferCleanup(th.DeleteInstance, CreateWatcherApplier(watcherTest.WatcherApplier, GetDefaultWatcherApplierSpec()))
		})
		It("should have input ready", func() {
			th.ExpectCondition(
				watcherTest.WatcherApplier,
				ConditionGetterFunc(WatcherApplierConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("should have memcached ready true", func() {
			th.ExpectCondition(
				watcherTest.WatcherApplier,
				ConditionGetterFunc(WatcherApplierConditionGetter),
				condition.MemcachedReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("should have config service input ready", func() {
			th.ExpectCondition(
				watcherTest.WatcherApplier,
				ConditionGetterFunc(WatcherApplierConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("should have cretaed the config secrete with the expected content", func() {
			th.ExpectCondition(
				watcherTest.WatcherApplier,
				ConditionGetterFunc(WatcherApplierConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
			// assert that the top level secret is created with proper content
			createdSecret := th.GetSecret(watcherTest.WatcherApplierSecret)
			Expect(createdSecret).ShouldNot(BeNil())
			Expect(createdSecret.Data["00-default.conf"]).ShouldNot(BeNil())

			// extract default config data
			configData := createdSecret.Data["00-default.conf"]
			Expect(configData).ShouldNot(BeNil())

			// indentaion is forced by use of raw literal
			expectedSections := []string{`
[cinder_client]
endpoint_type = internal`, `
[glance_client]
endpoint_type = internal`, `
[ironic_client]
endpoint_type = internal`, `
[keystone_client]
interface = internal`, `
[neutron_client]
endpoint_type = internal`, `
[nova_client]
endpoint_type = internal`, `
[placement_client]
interface = internal`, `
[oslo_messaging_notifications]

driver = messagingv2
transport_url = rabbit://rabbitmq-notification-secret/fake`, `
[oslo_messaging_rabbit]
amqp_durable_queues=false
amqp_auto_delete=false
heartbeat_in_pthread=false`,
			}
			for _, val := range expectedSections {
				Expect(string(configData)).Should(ContainSubstring(val))
			}
		})
	})
})
