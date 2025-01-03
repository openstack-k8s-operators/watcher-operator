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
	MinimalWatcherAPISpec = map[string]interface{}{
		"secret":            "osp-secret",
		"memcachedInstance": "memcached",
	}
)

var _ = Describe("WatcherAPI controller with minimal spec values", func() {
	When("A Watcher instance is created from minimal spec", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateWatcherAPI(watcherTest.WatcherAPI, MinimalWatcherAPISpec))
		})

		It("should have the Spec fields defaulted", func() {
			WatcherAPI := GetWatcherAPI(watcherTest.WatcherAPI)
			Expect(WatcherAPI.Spec.Secret).Should(Equal("osp-secret"))
			Expect(WatcherAPI.Spec.MemcachedInstance).Should(Equal("memcached"))
			Expect(WatcherAPI.Spec.PasswordSelectors).Should(Equal(watcherv1beta1.PasswordSelector{Service: "WatcherPassword"}))
		})

		It("should have the Status fields initialized", func() {
			WatcherAPI := GetWatcherAPI(watcherTest.WatcherAPI)
			Expect(WatcherAPI.Status.ObservedGeneration).To(Equal(int64(0)))
		})

		It("should have a finalizer", func() {
			// the reconciler loop adds the finalizer so we have to wait for
			// it to run
			Eventually(func() []string {
				return GetWatcherAPI(watcherTest.WatcherAPI).Finalizers
			}, timeout, interval).Should(ContainElement("openstack.org/watcherapi"))
		})

	})
})

var _ = Describe("WatcherAPI controller", func() {
	When("A WatcherAPI instance is created", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateWatcherAPI(watcherTest.WatcherAPI, GetDefaultWatcherAPISpec()))
		})

		It("should have the Spec fields defaulted", func() {
			WatcherAPI := GetWatcherAPI(watcherTest.WatcherAPI)
			Expect(WatcherAPI.Spec.Secret).Should(Equal("test-osp-secret"))
			Expect(WatcherAPI.Spec.MemcachedInstance).Should(Equal("memcached"))
		})

		It("should have the Status fields initialized", func() {
			WatcherAPI := GetWatcherAPI(watcherTest.WatcherAPI)
			Expect(WatcherAPI.Status.ObservedGeneration).To(Equal(int64(0)))
		})

		It("should have ReadyCondition false", func() {
			th.ExpectCondition(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("should have input not ready", func() {
			th.ExpectCondition(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("should have service config input unknown", func() {
			th.ExpectCondition(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionUnknown,
			)
		})

		It("should have a finalizer", func() {
			// the reconciler loop adds the finalizer so we have to wait for
			// it to run
			Eventually(func() []string {
				return GetWatcherAPI(watcherTest.WatcherAPI).Finalizers
			}, timeout, interval).Should(ContainElement("openstack.org/watcherapi"))
		})
	})
	When("the secret is created with all the expected fields and has all the required infra", func() {
		BeforeEach(func() {
			secret := th.CreateSecret(
				watcherTest.InternalTopLevelSecretName,
				map[string][]byte{
					"WatcherPassword": []byte("service-password"),
					"transport_url":   []byte("url"),
				},
			)
			DeferCleanup(k8sClient.Delete, ctx, secret)
			mariadb.CreateMariaDBDatabase(watcherTest.WatcherDatabaseName.Namespace, watcherTest.WatcherDatabaseName.Name, mariadbv1.MariaDBDatabaseSpec{})
			DeferCleanup(k8sClient.Delete, ctx, mariadb.GetMariaDBDatabase(watcherTest.WatcherDatabaseName))

			mariadb.SimulateMariaDBTLSDatabaseCompleted(watcherTest.WatcherDatabaseName)
			apiMariaDBAccount, apiMariaDBSecret := mariadb.CreateMariaDBAccountAndSecret(
				watcherTest.WatcherDatabaseAccount, mariadbv1.MariaDBAccountSpec{})
			DeferCleanup(k8sClient.Delete, ctx, apiMariaDBAccount)
			DeferCleanup(k8sClient.Delete, ctx, apiMariaDBSecret)
			DeferCleanup(th.DeleteInstance, CreateWatcherAPI(watcherTest.WatcherAPI, GetDefaultWatcherAPISpec()))
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(watcherTest.WatcherAPI.Namespace))
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.WatcherAPI.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
		})
		It("should have input ready", func() {
			th.ExpectCondition(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("should have memcached ready true", func() {
			th.ExpectCondition(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.MemcachedReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("should have config service input ready", func() {
			th.ExpectCondition(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("creates a deployment for the watcher-api service", func() {
			th.SimulateDeploymentReplicaReady(watcherTest.WatcherAPIDeployment)
			th.ExpectCondition(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)

			deployment := th.GetDeployment(watcherTest.WatcherAPIDeployment)
			Expect(deployment.Spec.Template.Spec.ServiceAccountName).To(Equal("watcher-sa"))
			Expect(int(*deployment.Spec.Replicas)).To(Equal(1))
			Expect(deployment.Spec.Template.Spec.Volumes).To(HaveLen(3))
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(2))
			Expect(deployment.Spec.Selector.MatchLabels).To(Equal(map[string]string{"service": "watcher-api"}))

			container := deployment.Spec.Template.Spec.Containers[0]
			Expect(container.VolumeMounts).To(HaveLen(1))
			Expect(container.Image).To(Equal("test://watcher"))

			container = deployment.Spec.Template.Spec.Containers[1]
			Expect(container.VolumeMounts).To(HaveLen(4))
			Expect(container.Image).To(Equal("test://watcher"))

			Expect(container.LivenessProbe.HTTPGet.Port.IntVal).To(Equal(int32(9322)))
			Expect(container.ReadinessProbe.HTTPGet.Port.IntVal).To(Equal(int32(9322)))
		})
	})
	When("the secret is created but missing fields", func() {
		BeforeEach(func() {
			secret := th.CreateSecret(
				watcherTest.InternalTopLevelSecretName,
				map[string][]byte{},
			)
			DeferCleanup(k8sClient.Delete, ctx, secret)
			mariadb.CreateMariaDBDatabase(watcherTest.WatcherDatabaseName.Namespace, watcherTest.WatcherDatabaseName.Name, mariadbv1.MariaDBDatabaseSpec{})
			DeferCleanup(k8sClient.Delete, ctx, mariadb.GetMariaDBDatabase(watcherTest.WatcherDatabaseName))

			mariadb.SimulateMariaDBTLSDatabaseCompleted(watcherTest.WatcherDatabaseName)
			apiMariaDBAccount, apiMariaDBSecret := mariadb.CreateMariaDBAccountAndSecret(
				watcherTest.WatcherDatabaseAccount, mariadbv1.MariaDBAccountSpec{})
			DeferCleanup(k8sClient.Delete, ctx, apiMariaDBAccount)
			DeferCleanup(k8sClient.Delete, ctx, apiMariaDBSecret)
			DeferCleanup(th.DeleteInstance, CreateWatcherAPI(watcherTest.WatcherAPI, GetDefaultWatcherAPISpec()))
		})
		It("should have input false", func() {
			errorString := fmt.Sprintf(
				condition.InputReadyErrorMessage,
				"field 'WatcherPassword' not found in secret/test-osp-secret",
			)
			th.ExpectConditionWithDetails(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				errorString,
			)
		})
		It("should have config service input unknown", func() {
			th.ExpectCondition(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionUnknown,
			)
		})
	})
	When("A WatcherAPI instance without secret is created", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateWatcherAPI(watcherTest.WatcherAPI, GetDefaultWatcherAPISpec()))
		})
		It("is missing the secret", func() {
			th.ExpectConditionWithDetails(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
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
					"WatcherPassword":   []byte("service-password"),
					"transport_url":     []byte("url"),
					"database_username": []byte("username"),
					"database_password": []byte("password"),
					"database_hostname": []byte("hostname"),
				},
			)
			DeferCleanup(k8sClient.Delete, ctx, secret)
			mariadb.CreateMariaDBDatabase(watcherTest.WatcherDatabaseName.Namespace, watcherTest.WatcherDatabaseName.Name, mariadbv1.MariaDBDatabaseSpec{})
			DeferCleanup(k8sClient.Delete, ctx, mariadb.GetMariaDBDatabase(watcherTest.WatcherDatabaseName))

			mariadb.SimulateMariaDBTLSDatabaseCompleted(watcherTest.WatcherDatabaseName)
			apiMariaDBAccount, apiMariaDBSecret := mariadb.CreateMariaDBAccountAndSecret(
				watcherTest.WatcherDatabaseAccount, mariadbv1.MariaDBAccountSpec{})
			DeferCleanup(k8sClient.Delete, ctx, apiMariaDBAccount)
			DeferCleanup(k8sClient.Delete, ctx, apiMariaDBSecret)
			DeferCleanup(th.DeleteInstance, CreateWatcherAPI(watcherTest.WatcherAPI, GetDefaultWatcherAPISpec()))
		})
		It("should have input ready true", func() {
			th.ExpectCondition(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("should have memcached ready false", func() {
			th.ExpectConditionWithDetails(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
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
					"WatcherPassword":   []byte("service-password"),
					"transport_url":     []byte("url"),
					"database_username": []byte("username"),
					"database_password": []byte("password"),
					"database_hostname": []byte("hostname"),
				},
			)
			DeferCleanup(k8sClient.Delete, ctx, secret)
			mariadb.CreateMariaDBDatabase(watcherTest.WatcherDatabaseName.Namespace, watcherTest.WatcherDatabaseName.Name, mariadbv1.MariaDBDatabaseSpec{})
			DeferCleanup(k8sClient.Delete, ctx, mariadb.GetMariaDBDatabase(watcherTest.WatcherDatabaseName))

			mariadb.SimulateMariaDBTLSDatabaseCompleted(watcherTest.WatcherDatabaseName)
			apiMariaDBAccount, apiMariaDBSecret := mariadb.CreateMariaDBAccountAndSecret(
				watcherTest.WatcherDatabaseAccount, mariadbv1.MariaDBAccountSpec{})
			DeferCleanup(k8sClient.Delete, ctx, apiMariaDBAccount)
			DeferCleanup(k8sClient.Delete, ctx, apiMariaDBSecret)
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.WatcherAPI.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
			DeferCleanup(th.DeleteInstance, CreateWatcherAPI(watcherTest.WatcherAPI, GetDefaultWatcherAPISpec()))

		})
		It("should have input ready true", func() {
			th.ExpectCondition(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("should have memcached ready true", func() {
			th.ExpectCondition(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
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
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				errorString,
			)
		})
	})
})
