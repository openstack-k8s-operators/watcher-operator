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
	MinimalWatcherDecisionEngineSpec = map[string]interface{}{
		"secret":            "osp-secret",
		"memcachedInstance": "memcached",
	}
)

var _ = Describe("WatcherDecisionEngine controller with minimal spec values", func() {
	When("A WatcherDecisionEngine instance is created from minimal spec", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateWatcherDecisionEngine(watcherTest.WatcherDecisionEngine, MinimalWatcherDecisionEngineSpec))
		})

		It("should have the Spec fields defaulted", func() {
			WatcherDecisionEngine := GetWatcherDecisionEngine(watcherTest.WatcherDecisionEngine)
			Expect(*(WatcherDecisionEngine.Spec.MemcachedInstance)).Should(Equal("memcached"))
			Expect(WatcherDecisionEngine.Spec.Secret).Should(Equal("osp-secret"))
			Expect(WatcherDecisionEngine.Spec.PasswordSelectors).Should(Equal(watcherv1beta1.PasswordSelector{Service: ptr.To("WatcherPassword")}))
		})

		It("should have the Status fields initialized", func() {
			WatcherDecisionEngine := GetWatcherDecisionEngine(watcherTest.WatcherDecisionEngine)
			Expect(WatcherDecisionEngine.Status.ObservedGeneration).To(Equal(int64(0)))
		})

		It("should have a finalizer", func() {
			// the reconciler loop adds the finalizer so we have to wait for
			// it to run
			Eventually(func() []string {
				return GetWatcherDecisionEngine(watcherTest.WatcherDecisionEngine).Finalizers
			}, timeout, interval).Should(ContainElement("openstack.org/watcherdecisionengine"))
		})

	})
})

var _ = Describe("WatcherDecisionEngine controller", func() {
	When("A WatcherDecisionEngine instance is created", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateWatcherDecisionEngine(watcherTest.WatcherDecisionEngine, GetDefaultWatcherDecisionEngineSpec()))
		})

		It("should have the Spec fields defaulted", func() {
			WatcherDecisionEngine := GetWatcherDecisionEngine(watcherTest.WatcherDecisionEngine)
			Expect(WatcherDecisionEngine.Spec.Secret).Should(Equal("test-osp-secret"))
			Expect(*(WatcherDecisionEngine.Spec.MemcachedInstance)).Should(Equal("memcached"))
		})

		It("should have the Status fields initialized", func() {
			WatcherDecisionEngine := GetWatcherDecisionEngine(watcherTest.WatcherDecisionEngine)
			Expect(WatcherDecisionEngine.Status.ObservedGeneration).To(Equal(int64(0)))
		})

		It("should have ReadyCondition false", func() {
			th.ExpectCondition(
				watcherTest.WatcherDecisionEngine,
				ConditionGetterFunc(WatcherDecisionEngineConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("should have input not ready", func() {
			th.ExpectCondition(
				watcherTest.WatcherDecisionEngine,
				ConditionGetterFunc(WatcherDecisionEngineConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("should have service config input unknown", func() {
			th.ExpectCondition(
				watcherTest.WatcherDecisionEngine,
				ConditionGetterFunc(WatcherDecisionEngineConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionUnknown,
			)
		})

		It("should have a finalizer", func() {
			// the reconciler loop adds the finalizer so we have to wait for
			// it to run
			Eventually(func() []string {
				return GetWatcherDecisionEngine(watcherTest.WatcherDecisionEngine).Finalizers
			}, timeout, interval).Should(ContainElement("openstack.org/watcherdecisionengine"))
		})

		It("should return false when using isReady method", func() {
			WatcherDecisionEngine := GetWatcherDecisionEngine(watcherTest.WatcherDecisionEngine)
			Expect(WatcherDecisionEngine.IsReady()).Should(BeFalse())
		})
	})
	When("the secret is created with all the expected fields", func() {
		var keystoneAPIName types.NamespacedName

		BeforeEach(func() {
			secret := CreateInternalTopLevelSecret()
			DeferCleanup(k8sClient.Delete, ctx, secret)

			prometheusSecret := th.CreateSecret(
				watcherTest.PrometheusSecretName,
				map[string][]byte{
					"host": []byte("prometheus.example.com"),
					"port": []byte("9090"),
				},
			)
			DeferCleanup(k8sClient.Delete, ctx, prometheusSecret)

			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					watcherTest.WatcherDecisionEngine.Namespace,
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
				watcherTest.WatcherDecisionEngine.Namespace,
				"watcher",
				mariadbv1.MariaDBDatabaseSpec{
					Name: "watcher",
				},
			)
			mariadb.SimulateMariaDBAccountCompleted(watcherTest.WatcherDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(watcherTest.WatcherDatabaseName)
			keystoneAPIName = keystone.CreateKeystoneAPI(watcherTest.WatcherDecisionEngine.Namespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPIName)
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.WatcherDecisionEngine.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
			DeferCleanup(th.DeleteInstance, CreateWatcherDecisionEngine(watcherTest.WatcherDecisionEngine, GetDefaultWatcherDecisionEngineSpec()))

		})
		It("should have input ready", func() {
			th.ExpectCondition(
				watcherTest.WatcherDecisionEngine,
				ConditionGetterFunc(WatcherDecisionEngineConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("should have memcached ready true", func() {
			th.ExpectCondition(
				watcherTest.WatcherDecisionEngine,
				ConditionGetterFunc(WatcherDecisionEngineConditionGetter),
				condition.MemcachedReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("should have config service input ready", func() {
			th.ExpectCondition(
				watcherTest.WatcherDecisionEngine,
				ConditionGetterFunc(WatcherDecisionEngineConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("should have cretaed the config secrete with the expected content", func() {
			th.ExpectCondition(
				watcherTest.WatcherDecisionEngine,
				ConditionGetterFunc(WatcherDecisionEngineConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
			// assert that the top level secret is created with proper content
			createdSecret := th.GetSecret(watcherTest.WatcherDecisionEngineSecret)
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
[watcher_cluster_data_model_collectors.compute]
period = 900`, `
[watcher_cluster_data_model_collectors.baremetal]
period = 900`, `
[watcher_cluster_data_model_collectors.storage]
period = 900`, `
[oslo_messaging_notifications]

driver = noop`, `
[oslo_messaging_rabbit]
amqp_durable_queues=false
amqp_auto_delete=false
heartbeat_in_pthread=false`,
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
		It("creates a statefulset for the watcher-decision-engine service", func() {
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherDecisionEngineStatefulSet)
			th.ExpectCondition(
				watcherTest.WatcherDecisionEngine,
				ConditionGetterFunc(WatcherDecisionEngineConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				watcherTest.WatcherDecisionEngine,
				ConditionGetterFunc(WatcherDecisionEngineConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			statefulset := th.GetStatefulSet(watcherTest.WatcherDecisionEngineStatefulSet)
			Expect(statefulset.Spec.Template.Spec.ServiceAccountName).To(Equal("watcher-sa"))
			Expect(int(*statefulset.Spec.Replicas)).To(Equal(1))
			Expect(statefulset.Spec.Template.Spec.Volumes).To(HaveLen(3))
			Expect(statefulset.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(statefulset.Spec.Selector.MatchLabels).To(Equal(map[string]string{"service": "watcher-decision-engine"}))

			container := statefulset.Spec.Template.Spec.Containers[0]
			Expect(container.VolumeMounts).To(HaveLen(4))
			Expect(container.Image).To(Equal("test://watcher"))

			probeCmd := []string{
				"/usr/bin/pgrep", "-f", "-r", "DRST", "watcher-decision-engine",
			}
			Expect(container.StartupProbe.Exec.Command).To(Equal(probeCmd))
			Expect(container.LivenessProbe.Exec.Command).To(Equal(probeCmd))
			Expect(container.ReadinessProbe.Exec.Command).To(Equal(probeCmd))
		})
		It("should return true when using isReady method", func() {
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherDecisionEngineStatefulSet)

			th.ExpectCondition(
				watcherTest.WatcherDecisionEngine,
				ConditionGetterFunc(WatcherDecisionEngineConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			WatcherDecisionEngine := GetWatcherDecisionEngine(watcherTest.WatcherDecisionEngine)
			Expect(WatcherDecisionEngine.IsReady()).Should(BeTrue())
		})
		It("updates the KeystoneAuthURL if keystone internal endpoint changes", func() {
			newInternalEndpoint := "https://keystone-internal"

			keystone.UpdateKeystoneAPIEndpoint(keystoneAPIName, "internal", newInternalEndpoint)
			logger.Info("Reconfigured")

			th.ExpectCondition(
				watcherTest.WatcherDecisionEngine,
				ConditionGetterFunc(WatcherDecisionEngineConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				confSecret := th.GetSecret(watcherTest.WatcherDecisionEngineConfigSecret)
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
			DeferCleanup(th.DeleteInstance, CreateWatcherDecisionEngine(watcherTest.WatcherDecisionEngine, GetDefaultWatcherDecisionEngineSpec()))
		})
		It("should have input false", func() {
			errorString := fmt.Sprintf(
				condition.InputReadyErrorMessage,
				"field 'WatcherPassword' not found in secret/test-osp-secret",
			)
			th.ExpectConditionWithDetails(
				watcherTest.WatcherDecisionEngine,
				ConditionGetterFunc(WatcherDecisionEngineConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				errorString,
			)
		})
		It("should have config service input unknown", func() {
			th.ExpectCondition(
				watcherTest.WatcherDecisionEngine,
				ConditionGetterFunc(WatcherDecisionEngineConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionUnknown,
			)
		})
	})
	When("WatcherDecisionEngine is created with a wrong topologyRef", func() {
		BeforeEach(func() {
			secret := CreateInternalTopLevelSecret()
			DeferCleanup(k8sClient.Delete, ctx, secret)
			prometheusSecret := th.CreateSecret(
				watcherTest.PrometheusSecretName,
				map[string][]byte{
					"host": []byte("prometheus.example.com"),
					"port": []byte("9090"),
				},
			)
			DeferCleanup(k8sClient.Delete, ctx, prometheusSecret)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					watcherTest.WatcherDecisionEngine.Namespace,
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
				watcherTest.WatcherDecisionEngine.Namespace,
				"watcher",
				mariadbv1.MariaDBDatabaseSpec{
					Name: "watcher",
				},
			)
			mariadb.SimulateMariaDBAccountCompleted(watcherTest.WatcherDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(watcherTest.WatcherDatabaseName)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(watcherTest.WatcherDecisionEngine.Namespace))
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.WatcherDecisionEngine.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
			spec := GetDefaultWatcherDecisionEngineSpec()
			spec["topologyRef"] = map[string]interface{}{"name": "foo"}
			DeferCleanup(th.DeleteInstance, CreateWatcherDecisionEngine(watcherTest.WatcherDecisionEngine, spec))
		})
		It("points to a non existing topology CR", func() {
			th.ExpectCondition(
				watcherTest.WatcherDecisionEngine,
				ConditionGetterFunc(WatcherDecisionEngineConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				watcherTest.WatcherDecisionEngine,
				ConditionGetterFunc(WatcherDecisionEngineConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})
	})

	When("WatcherDecisionEngine is created with topology", func() {
		var topologyRefDecEng topologyv1.TopoRef
		var topologyRefAlt topologyv1.TopoRef
		var expectedTopologySpec []corev1.TopologySpreadConstraint
		BeforeEach(func() {
			var topologySpec map[string]interface{}
			// Build the topology Spec
			topologySpec, expectedTopologySpec = GetSampleTopologySpec("watcher-decision-engine")
			_ = expectedTopologySpec
			// Create Test Topologies
			_, topologyRefDecEng = infra.CreateTopology(
				types.NamespacedName{
					Namespace: namespace,
					Name:      "watcherdecisionengine"},
				topologySpec)
			_, topologyRefAlt = infra.CreateTopology(
				types.NamespacedName{
					Namespace: namespace,
					Name:      "watcher"},
				topologySpec)

			secret := CreateInternalTopLevelSecret()
			DeferCleanup(k8sClient.Delete, ctx, secret)
			prometheusSecret := th.CreateSecret(
				watcherTest.PrometheusSecretName,
				map[string][]byte{
					"host": []byte("prometheus.example.com"),
					"port": []byte("9090"),
				},
			)
			DeferCleanup(k8sClient.Delete, ctx, prometheusSecret)
			spec := GetDefaultWatcherDecisionEngineSpec()
			spec["topologyRef"] = map[string]interface{}{"name": topologyRefDecEng.Name}
			DeferCleanup(th.DeleteInstance, CreateWatcherDecisionEngine(watcherTest.WatcherDecisionEngine, spec))
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(watcherTest.WatcherDecisionEngine.Namespace))
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.WatcherDecisionEngine.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					watcherTest.WatcherDecisionEngine.Namespace,
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
				watcherTest.WatcherDecisionEngine.Namespace,
				"watcher",
				mariadbv1.MariaDBDatabaseSpec{
					Name: "watcher",
				},
			)
			mariadb.SimulateMariaDBAccountCompleted(watcherTest.WatcherDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(watcherTest.WatcherDatabaseName)
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherDecisionEngineStatefulSet)

		})
		It("sets lastAppliedTopology field in WatcherDecisionEngine topology .Status", func() {

			WatcherDecisionEngine := GetWatcherDecisionEngine(watcherTest.WatcherDecisionEngine)

			Expect(WatcherDecisionEngine.Status.LastAppliedTopology).ToNot(BeNil())
			Expect(WatcherDecisionEngine.Status.LastAppliedTopology).To(Equal(&topologyRefDecEng))

			th.ExpectCondition(
				watcherTest.WatcherDecisionEngine,
				ConditionGetterFunc(WatcherDecisionEngineConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				ss := th.GetStatefulSet(watcherTest.WatcherDecisionEngineStatefulSet)
				podTemplate := ss.Spec.Template.Spec
				g.Expect(podTemplate.TopologySpreadConstraints).ToNot(BeNil())
				// No default Pod Antiaffinity is applied
				g.Expect(podTemplate.Affinity).To(BeNil())
			}, timeout, interval).Should(Succeed())

			// Check finalizer
			Eventually(func(g Gomega) {
				tp := infra.GetTopology(types.NamespacedName{
					Name:      topologyRefDecEng.Name,
					Namespace: topologyRefDecEng.Namespace,
				})
				finalizers := tp.GetFinalizers()
				g.Expect(finalizers).To(ContainElement(
					fmt.Sprintf("openstack.org/watcherdecisionengine-%s", watcherTest.WatcherDecisionEngine.Name)))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				tpAlt := infra.GetTopology(types.NamespacedName{
					Name:      topologyRefAlt.Name,
					Namespace: topologyRefAlt.Namespace,
				})
				finalizers := tpAlt.GetFinalizers()
				g.Expect(finalizers).ToNot(ContainElement(
					fmt.Sprintf("openstack.org/watcherdecisionengine-%s", watcherTest.WatcherDecisionEngine.Name)))
			}, timeout, interval).Should(Succeed())
		})
		It("updates lastAppliedTopology in WatcherDecisionEngine .Status", func() {
			Eventually(func(g Gomega) {
				WatcherDecisionEngine := GetWatcherDecisionEngine(watcherTest.WatcherDecisionEngine)
				WatcherDecisionEngine.Spec.TopologyRef.Name = topologyRefAlt.Name
				g.Expect(k8sClient.Update(ctx, WatcherDecisionEngine)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				WatcherDecisionEngine := GetWatcherDecisionEngine(watcherTest.WatcherDecisionEngine)
				g.Expect(WatcherDecisionEngine.Status.LastAppliedTopology).ToNot(BeNil())
				g.Expect(WatcherDecisionEngine.Status.LastAppliedTopology).To(Equal(&topologyRefAlt))
			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				watcherTest.WatcherDecisionEngine,
				ConditionGetterFunc(WatcherDecisionEngineConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				ss := th.GetStatefulSet(watcherTest.WatcherDecisionEngine)
				podTemplate := ss.Spec.Template.Spec
				g.Expect(podTemplate.TopologySpreadConstraints).ToNot(BeNil())
				// No default Pod Antiaffinity is applied
				g.Expect(podTemplate.Affinity).To(BeNil())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				ss := th.GetStatefulSet(watcherTest.WatcherDecisionEngine)
				podTemplate := ss.Spec.Template.Spec
				g.Expect(podTemplate.TopologySpreadConstraints).To(Equal(expectedTopologySpec))
			}, timeout, interval).Should(Succeed())

			// Check finalizer is set to topologyRefAlt and is not set to
			// topologyRef
			Eventually(func(g Gomega) {
				tp := infra.GetTopology(types.NamespacedName{
					Name:      topologyRefDecEng.Name,
					Namespace: topologyRefDecEng.Namespace,
				})
				finalizers := tp.GetFinalizers()
				g.Expect(finalizers).ToNot(ContainElement(
					fmt.Sprintf("openstack.org/watcherdecisionengine-%s", watcherTest.WatcherDecisionEngine.Name)))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				tpAlt := infra.GetTopology(types.NamespacedName{
					Name:      topologyRefAlt.Name,
					Namespace: topologyRefAlt.Namespace,
				})
				finalizers := tpAlt.GetFinalizers()
				g.Expect(finalizers).To(ContainElement(
					fmt.Sprintf("openstack.org/watcherdecisionengine-%s", watcherTest.WatcherDecisionEngine.Name)))
			}, timeout, interval).Should(Succeed())
		})
		It("removes topologyRef from WatcherDecisionEngine spec", func() {
			Eventually(func(g Gomega) {
				WatcherDecisionEngine := GetWatcherDecisionEngine(watcherTest.WatcherDecisionEngine)
				WatcherDecisionEngine.Spec.TopologyRef = nil
				g.Expect(k8sClient.Update(ctx, WatcherDecisionEngine)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				WatcherDecisionEngine := GetWatcherDecisionEngine(watcherTest.WatcherDecisionEngine)
				g.Expect(WatcherDecisionEngine.Status.LastAppliedTopology).Should(BeNil())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				ss := th.GetStatefulSet(watcherTest.WatcherDecisionEngine)
				podTemplate := ss.Spec.Template.Spec
				g.Expect(podTemplate.TopologySpreadConstraints).To(BeNil())
				// Default Pod AntiAffinity is applied
				g.Expect(podTemplate.Affinity).ToNot(BeNil())
			}, timeout, interval).Should(Succeed())

			// Check finalizer is not present anymore
			Eventually(func(g Gomega) {
				tp := infra.GetTopology(types.NamespacedName{
					Name:      topologyRefDecEng.Name,
					Namespace: topologyRefDecEng.Namespace,
				})
				finalizers := tp.GetFinalizers()
				g.Expect(finalizers).ToNot(ContainElement(
					fmt.Sprintf("openstack.org/watcherdecisionengine-%s", watcherTest.WatcherDecisionEngine.Name)))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				tpAlt := infra.GetTopology(types.NamespacedName{
					Name:      topologyRefAlt.Name,
					Namespace: topologyRefAlt.Namespace,
				})
				finalizers := tpAlt.GetFinalizers()
				g.Expect(finalizers).ToNot(ContainElement(
					fmt.Sprintf("openstack.org/watcherdecisionengine-%s", watcherTest.WatcherDecisionEngine.Name)))
			}, timeout, interval).Should(Succeed())
		})
	})
	When("the secret is created with a notification_url fields", func() {
		var keystoneAPIName types.NamespacedName

		BeforeEach(func() {
			secret := CreateInternalTopLevelSecretNotification()
			DeferCleanup(k8sClient.Delete, ctx, secret)

			prometheusSecret := th.CreateSecret(
				watcherTest.PrometheusSecretName,
				map[string][]byte{
					"host": []byte("prometheus.example.com"),
					"port": []byte("9090"),
				},
			)
			DeferCleanup(k8sClient.Delete, ctx, prometheusSecret)

			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					watcherTest.WatcherDecisionEngine.Namespace,
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
				watcherTest.WatcherDecisionEngine.Namespace,
				"watcher",
				mariadbv1.MariaDBDatabaseSpec{
					Name: "watcher",
				},
			)
			mariadb.SimulateMariaDBAccountCompleted(watcherTest.WatcherDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(watcherTest.WatcherDatabaseName)
			keystoneAPIName = keystone.CreateKeystoneAPI(watcherTest.WatcherDecisionEngine.Namespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPIName)
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.WatcherDecisionEngine.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
			DeferCleanup(th.DeleteInstance, CreateWatcherDecisionEngine(watcherTest.WatcherDecisionEngine, GetDefaultWatcherDecisionEngineSpec()))

		})
		It("should have input ready", func() {
			th.ExpectCondition(
				watcherTest.WatcherDecisionEngine,
				ConditionGetterFunc(WatcherDecisionEngineConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("should have memcached ready true", func() {
			th.ExpectCondition(
				watcherTest.WatcherDecisionEngine,
				ConditionGetterFunc(WatcherDecisionEngineConditionGetter),
				condition.MemcachedReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("should have config service input ready", func() {
			th.ExpectCondition(
				watcherTest.WatcherDecisionEngine,
				ConditionGetterFunc(WatcherDecisionEngineConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("should have cretaed the config secrete with the expected content", func() {
			th.ExpectCondition(
				watcherTest.WatcherDecisionEngine,
				ConditionGetterFunc(WatcherDecisionEngineConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
			// assert that the top level secret is created with proper content
			createdSecret := th.GetSecret(watcherTest.WatcherDecisionEngineSecret)
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
[watcher_cluster_data_model_collectors.compute]
period = 900`, `
[watcher_cluster_data_model_collectors.baremetal]
period = 900`, `
[watcher_cluster_data_model_collectors.storage]
period = 900`, `
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

	When("A WatcherDecisionEngine instance is created with MTLS memcached", func() {
		BeforeEach(func() {
			// Create the required secret for WatcherDecisionEngine
			secret := CreateInternalTopLevelSecret()
			DeferCleanup(k8sClient.Delete, ctx, secret)

			// Create prometheus secret
			prometheusSecret := th.CreateSecret(
				watcherTest.PrometheusSecretName,
				map[string][]byte{
					"host": []byte("prometheus.example.com"),
					"port": []byte("9090"),
				},
			)
			DeferCleanup(k8sClient.Delete, ctx, prometheusSecret)

			mariadb.CreateMariaDBDatabase(watcherTest.WatcherDatabaseName.Namespace, watcherTest.WatcherDatabaseName.Name, mariadbv1.MariaDBDatabaseSpec{})
			DeferCleanup(k8sClient.Delete, ctx, mariadb.GetMariaDBDatabase(watcherTest.WatcherDatabaseName))

			mariadb.SimulateMariaDBDatabaseCompleted(watcherTest.WatcherDatabaseName)
			decisionEngineMariaDBAccount, decisionEngineMariaDBSecret := mariadb.CreateMariaDBAccountAndSecret(
				watcherTest.WatcherDatabaseAccount, mariadbv1.MariaDBAccountSpec{})
			DeferCleanup(k8sClient.Delete, ctx, decisionEngineMariaDBAccount)
			DeferCleanup(k8sClient.Delete, ctx, decisionEngineMariaDBSecret)

			memcachedSpec := infra.GetDefaultMemcachedSpec()
			// Create Memcached with MTLS auth
			DeferCleanup(infra.DeleteMemcached, infra.CreateMTLSMemcached(watcherTest.WatcherDecisionEngine.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMTLSMemcachedReady(watcherTest.MemcachedNamespace)

			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(watcherTest.WatcherDecisionEngine.Namespace))
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					watcherTest.WatcherDecisionEngine.Namespace,
					"openstack",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)

			DeferCleanup(th.DeleteInstance, CreateWatcherDecisionEngine(watcherTest.WatcherDecisionEngine, GetDefaultWatcherDecisionEngineSpec()))
		})

		It("should have MTLS volumes mounted in StatefulSet", func() {
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherDecisionEngineStatefulSet)
			Eventually(func(g Gomega) {
				ss := th.GetStatefulSet(watcherTest.WatcherDecisionEngineStatefulSet)

				// Check that MTLS volume is present
				mtlsVolumeFound := false
				for _, volume := range ss.Spec.Template.Spec.Volumes {
					if volume.Name == "cert-memcached-mtls" {
						mtlsVolumeFound = true
						g.Expect(volume.Secret).ToNot(BeNil())
						g.Expect(volume.Secret.SecretName).To(Equal("cert-memcached-mtls"))
						break
					}
				}
				g.Expect(mtlsVolumeFound).To(BeTrue(), "MTLS volume should be mounted")

				// Check that MTLS volume mounts are present
				for _, container := range ss.Spec.Template.Spec.Containers {
					if container.Name == "watcher-decision-engine" {
						mtlsVolumeMountFound := false
						for _, volumeMount := range container.VolumeMounts {
							if volumeMount.Name == "cert-memcached-mtls" {
								mtlsVolumeMountFound = true
								break
							}
						}
						g.Expect(mtlsVolumeMountFound).To(BeTrue(), "MTLS volume mount should be present in watcher-decision-engine container")
					}
				}
			}, timeout, interval).Should(Succeed())
		})

		It("should have MTLS configuration in config secret", func() {
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherDecisionEngineStatefulSet)
			Eventually(func(g Gomega) {
				configSecret := th.GetSecret(watcherTest.WatcherDecisionEngineConfigSecret)
				g.Expect(configSecret).ToNot(BeNil())

				configData, exists := configSecret.Data["00-default.conf"]
				g.Expect(exists).To(BeTrue())
				configString := string(configData)

				// Check for MTLS configuration in keystone_authtoken section
				g.Expect(configString).To(ContainSubstring("memcache_tls_certfile"))
				g.Expect(configString).To(ContainSubstring("memcache_tls_keyfile"))
				g.Expect(configString).To(ContainSubstring("memcache_tls_cafile"))
				g.Expect(configString).To(ContainSubstring("memcache_tls_enabled = true"))

				// Check for MTLS configuration in cache section
				g.Expect(configString).To(ContainSubstring("tls_certfile"))
				g.Expect(configString).To(ContainSubstring("tls_keyfile"))
				g.Expect(configString).To(ContainSubstring("tls_cafile"))
			}, timeout, interval).Should(Succeed())
		})
	})

	When("the secret is created with quorumqueues=true in the top level secret", func() {
		var keystoneAPIName types.NamespacedName

		BeforeEach(func() {
			secret := CreateInternalTopLevelSecretQuorum()
			DeferCleanup(k8sClient.Delete, ctx, secret)

			prometheusSecret := th.CreateSecret(
				watcherTest.PrometheusSecretName,
				map[string][]byte{
					"host": []byte("prometheus.example.com"),
					"port": []byte("9090"),
				},
			)
			DeferCleanup(k8sClient.Delete, ctx, prometheusSecret)

			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					watcherTest.WatcherDecisionEngine.Namespace,
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
				watcherTest.WatcherDecisionEngine.Namespace,
				"watcher",
				mariadbv1.MariaDBDatabaseSpec{
					Name: "watcher",
				},
			)
			mariadb.SimulateMariaDBAccountCompleted(watcherTest.WatcherDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(watcherTest.WatcherDatabaseName)
			keystoneAPIName = keystone.CreateKeystoneAPI(watcherTest.WatcherDecisionEngine.Namespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPIName)
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.WatcherDecisionEngine.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
			DeferCleanup(th.DeleteInstance, CreateWatcherDecisionEngine(watcherTest.WatcherDecisionEngine, GetDefaultWatcherDecisionEngineSpec()))
		})
		It("should have input ready", func() {
			th.ExpectCondition(
				watcherTest.WatcherDecisionEngine,
				ConditionGetterFunc(WatcherDecisionEngineConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("should have memcached ready true", func() {
			th.ExpectCondition(
				watcherTest.WatcherDecisionEngine,
				ConditionGetterFunc(WatcherDecisionEngineConditionGetter),
				condition.MemcachedReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("should have config service input ready", func() {
			th.ExpectCondition(
				watcherTest.WatcherDecisionEngine,
				ConditionGetterFunc(WatcherDecisionEngineConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("should have the expected config secret content with quorum queue configuration", func() {
			createdSecret := th.GetSecret(watcherTest.WatcherDecisionEngineSecret)
			Expect(createdSecret).ShouldNot(BeNil())
			Expect(createdSecret.Data["00-default.conf"]).ShouldNot(BeNil())

			// extract default config data
			configData := createdSecret.Data["00-default.conf"]
			Expect(configData).ShouldNot(BeNil())

			// Only check quorum queue specific configuration
			expectedSections := []string{`
[oslo_messaging_rabbit]
rabbit_quorum_queue=true
rabbit_transient_quorum_queue=true
amqp_durable_queues=true`,
			}
			for _, val := range expectedSections {
				Expect(string(configData)).Should(ContainSubstring(val))
			}
		})
	})

	When("the secret is created with quorumqueues=false in the top level secret", func() {
		var keystoneAPIName types.NamespacedName
		BeforeEach(func() {
			secret := CreateInternalTopLevelSecret() // This has quorumqueues=false by default
			DeferCleanup(k8sClient.Delete, ctx, secret)

			prometheusSecret := th.CreateSecret(
				watcherTest.PrometheusSecretName,
				map[string][]byte{
					"host": []byte("prometheus.example.com"),
					"port": []byte("9090"),
				},
			)
			DeferCleanup(k8sClient.Delete, ctx, prometheusSecret)

			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					watcherTest.WatcherDecisionEngine.Namespace,
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
				watcherTest.WatcherDecisionEngine.Namespace,
				"watcher",
				mariadbv1.MariaDBDatabaseSpec{
					Name: "watcher",
				},
			)
			mariadb.SimulateMariaDBAccountCompleted(watcherTest.WatcherDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(watcherTest.WatcherDatabaseName)
			keystoneAPIName = keystone.CreateKeystoneAPI(watcherTest.WatcherDecisionEngine.Namespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPIName)
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.WatcherDecisionEngine.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
			DeferCleanup(th.DeleteInstance, CreateWatcherDecisionEngine(watcherTest.WatcherDecisionEngine, GetDefaultWatcherDecisionEngineSpec()))

		})
		It("should have input ready", func() {
			th.ExpectCondition(
				watcherTest.WatcherDecisionEngine,
				ConditionGetterFunc(WatcherDecisionEngineConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("should have config service input ready", func() {
			th.ExpectCondition(
				watcherTest.WatcherDecisionEngine,
				ConditionGetterFunc(WatcherDecisionEngineConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("should have the expected config secret content with default quorum queue settings", func() {

			createdSecret := th.GetSecret(watcherTest.WatcherDecisionEngineSecret)
			Expect(createdSecret).ShouldNot(BeNil())
			Expect(createdSecret.Data["00-default.conf"]).ShouldNot(BeNil())

			// extract default config data
			configData := createdSecret.Data["00-default.conf"]
			Expect(configData).ShouldNot(BeNil())

			expectedSections := []string{`
[oslo_messaging_rabbit]
amqp_durable_queues=false
amqp_auto_delete=false
heartbeat_in_pthread=false`,
			}
			for _, val := range expectedSections {
				Expect(string(configData)).Should(ContainSubstring(val))
			}

			// Ensure quorum queue settings are NOT present
			Expect(string(configData)).Should(Not(ContainSubstring("rabbit_quorum_queue=true")))
			Expect(string(configData)).Should(Not(ContainSubstring("rabbit_transient_quorum_queue=true")))
			Expect(string(configData)).Should(Not(ContainSubstring("amqp_durable_queues=true")))
		})
	})

	When("the secret is updated to change quorum queues configuration", func() {
		var keystoneAPIName types.NamespacedName
		BeforeEach(func() {
			secret := CreateInternalTopLevelSecretQuorum() // Start with quorum enabled
			DeferCleanup(k8sClient.Delete, ctx, secret)

			prometheusSecret := th.CreateSecret(
				watcherTest.PrometheusSecretName,
				map[string][]byte{
					"host": []byte("prometheus.example.com"),
					"port": []byte("9090"),
				},
			)
			DeferCleanup(k8sClient.Delete, ctx, prometheusSecret)

			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					watcherTest.WatcherDecisionEngine.Namespace,
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
				watcherTest.WatcherDecisionEngine.Namespace,
				"watcher",
				mariadbv1.MariaDBDatabaseSpec{
					Name: "watcher",
				},
			)
			mariadb.SimulateMariaDBAccountCompleted(watcherTest.WatcherDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(watcherTest.WatcherDatabaseName)
			keystoneAPIName = keystone.CreateKeystoneAPI(watcherTest.WatcherDecisionEngine.Namespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPIName)
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.WatcherDecisionEngine.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
			DeferCleanup(th.DeleteInstance, CreateWatcherDecisionEngine(watcherTest.WatcherDecisionEngine, GetDefaultWatcherDecisionEngineSpec()))

		})

		It("should reconfigure when quorum queues are disabled", func() {
			// First verify quorum queues are enabled initially
			Eventually(func(g Gomega) {
				createdSecret := th.GetSecret(watcherTest.WatcherDecisionEngineSecret)
				g.Expect(createdSecret).ShouldNot(BeNil())
				configData := string(createdSecret.Data["00-default.conf"])
				g.Expect(configData).Should(ContainSubstring("rabbit_quorum_queue=true"))
				g.Expect(configData).Should(ContainSubstring("rabbit_transient_quorum_queue=true"))
				g.Expect(configData).Should(ContainSubstring("amqp_durable_queues=true"))
			}, timeout, interval).Should(Succeed())

			// Update the secret to disable quorum queues
			Eventually(func(g Gomega) {
				secret := &corev1.Secret{}
				g.Expect(k8sClient.Get(ctx, watcherTest.InternalTopLevelSecretName, secret)).Should(Succeed())
				secret.Data["quorumqueues"] = []byte("false")
				g.Expect(k8sClient.Update(ctx, secret)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			// Verify configuration is updated to default settings
			Eventually(func(g Gomega) {
				createdSecret := th.GetSecret(watcherTest.WatcherDecisionEngineSecret)
				g.Expect(createdSecret).ShouldNot(BeNil())
				configData := string(createdSecret.Data["00-default.conf"])
				g.Expect(configData).Should(ContainSubstring("amqp_durable_queues=false"))
				g.Expect(configData).Should(ContainSubstring("amqp_auto_delete=false"))
				g.Expect(configData).Should(ContainSubstring("heartbeat_in_pthread=false"))
				// Ensure quorum queue settings are removed
				g.Expect(configData).Should(Not(ContainSubstring("rabbit_quorum_queue=true")))
				g.Expect(configData).Should(Not(ContainSubstring("rabbit_transient_quorum_queue=true")))
				g.Expect(configData).Should(Not(ContainSubstring("amqp_durable_queues=true")))
			}, timeout, interval).Should(Succeed())
		})
	})
})
