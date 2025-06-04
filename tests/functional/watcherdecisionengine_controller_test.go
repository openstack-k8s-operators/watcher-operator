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
	"github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
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
			Expect(WatcherDecisionEngine.Spec.MemcachedInstance).Should(Equal("memcached"))
			Expect(WatcherDecisionEngine.Spec.Secret).Should(Equal("osp-secret"))
			Expect(WatcherDecisionEngine.Spec.PasswordSelectors).Should(Equal(watcherv1beta1.PasswordSelector{Service: "WatcherPassword"}))
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
			Expect(WatcherDecisionEngine.Spec.MemcachedInstance).Should(Equal("memcached"))
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
				},
			)
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
period = 900`,
			}
			for _, val := range expectedSections {
				Expect(string(configData)).Should(ContainSubstring(val))
			}

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
				},
			)
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
				},
			)
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
				v1beta1.MariaDBAccountSpec{
					UserName: "watcher",
				},
			)
			mariadb.CreateMariaDBDatabase(
				watcherTest.WatcherDecisionEngine.Namespace,
				"watcher",
				v1beta1.MariaDBDatabaseSpec{
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

})
