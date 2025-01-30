package functional

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	//revive:disable-next-line:dot-imports
	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
	watcherv1beta1 "github.com/openstack-k8s-operators/watcher-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
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
	})
	When("the secret is created with all the expected fields", func() {
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
})
