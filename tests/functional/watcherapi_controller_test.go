package functional

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	//revive:disable-next-line:dot-imports
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
	watcherv1beta1 "github.com/openstack-k8s-operators/watcher-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/watcher-operator/pkg/watcher"
	corev1 "k8s.io/api/core/v1"
)

var (
	MinimalWatcherAPISpec = map[string]interface{}{
		"secret":           "osp-secret",
		"databaseInstance": "openstack",
	}
)

var _ = Describe("WatcherAPI controller with minimal spec values", func() {
	When("A Watcher instance is created from minimal spec", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateWatcherAPI(watcherTest.Instance, MinimalWatcherAPISpec))
		})

		It("should have the Spec fields defaulted", func() {
			WatcherAPI := GetWatcherAPI(watcherTest.Instance)
			Expect(WatcherAPI.Spec.DatabaseInstance).Should(Equal("openstack"))
			Expect(WatcherAPI.Spec.DatabaseAccount).Should(Equal("watcher"))
			Expect(WatcherAPI.Spec.Secret).Should(Equal("osp-secret"))
			Expect(WatcherAPI.Spec.PasswordSelectors).Should(Equal(watcherv1beta1.PasswordSelector{Service: "WatcherPassword"}))
		})

		It("should have the Status fields initialized", func() {
			WatcherAPI := GetWatcherAPI(watcherTest.Instance)
			Expect(WatcherAPI.Status.ObservedGeneration).To(Equal(int64(0)))
		})

		It("should have a finalizer", func() {
			// the reconciler loop adds the finalizer so we have to wait for
			// it to run
			Eventually(func() []string {
				return GetWatcherAPI(watcherTest.Instance).Finalizers
			}, timeout, interval).Should(ContainElement("openstack.org/watcherapi"))
		})

	})
})

var _ = Describe("WatcherAPI controller", func() {
	When("A WatcherAPI instance is created", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateWatcherAPI(watcherTest.Instance, GetDefaultWatcherAPISpec()))
		})

		It("should have the Spec fields defaulted", func() {
			WatcherAPI := GetWatcherAPI(watcherTest.Instance)
			Expect(WatcherAPI.Spec.DatabaseInstance).Should(Equal("openstack"))
			Expect(WatcherAPI.Spec.DatabaseAccount).Should(Equal("watcher"))
			Expect(WatcherAPI.Spec.Secret).Should(Equal("test-osp-secret"))
		})

		It("should have the Status fields initialized", func() {
			WatcherAPI := GetWatcherAPI(watcherTest.Instance)
			Expect(WatcherAPI.Status.ObservedGeneration).To(Equal(int64(0)))
		})

		It("should have ReadyCondition false", func() {
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("should have input not ready", func() {
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("should have service config input unknown", func() {
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionUnknown,
			)
		})

		It("should have a finalizer", func() {
			// the reconciler loop adds the finalizer so we have to wait for
			// it to run
			Eventually(func() []string {
				return GetWatcherAPI(watcherTest.Instance).Finalizers
			}, timeout, interval).Should(ContainElement("openstack.org/watcherapi"))
		})
	})
	When("the secret is created with all the expected fields", func() {
		BeforeEach(func() {
			secret := th.CreateSecret(
				watcherTest.InternalTopLevelSecretName,
				map[string][]byte{
					"WatcherPassword": []byte("service-password"),
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
			DeferCleanup(th.DeleteInstance, CreateWatcherAPI(watcherTest.Instance, GetDefaultWatcherAPISpec()))
		})
		It("should have input ready", func() {
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("should have config service input ready", func() {
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
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
			DeferCleanup(th.DeleteInstance, CreateWatcherAPI(watcherTest.Instance, GetDefaultWatcherAPISpec()))
		})
		It("should have input false", func() {
			errorString := fmt.Sprintf(
				condition.InputReadyErrorMessage,
				"field 'WatcherPassword' not found in secret/test-osp-secret",
			)
			th.ExpectConditionWithDetails(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				errorString,
			)
		})
		It("should have config service input unknown", func() {
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionUnknown,
			)
		})
	})
	When("A WatcherAPI instance without secret is created", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateWatcherAPI(watcherTest.Instance, GetDefaultWatcherAPISpec()))
		})
		It("is missing the secret", func() {
			th.ExpectConditionWithDetails(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				condition.InputReadyWaitingMessage,
			)
		})
	})
	When("A WatcherAPI instance without a database but with a secret is created", func() {
		BeforeEach(func() {
			secret := th.CreateSecret(
				watcherTest.InternalTopLevelSecretName,
				map[string][]byte{
					"WatcherPassword": []byte("service-password"),
				},
			)
			DeferCleanup(k8sClient.Delete, ctx, secret)
			DeferCleanup(th.DeleteInstance, CreateWatcherAPI(watcherTest.Instance, GetDefaultWatcherAPISpec()))
		})
		It("should have input not ready", func() {
			WatcherAPI := GetWatcherAPI(watcherTest.Instance)
			customErrorString := fmt.Sprintf(
				"couldn't get database %s and account %s",
				watcher.DatabaseCRName,
				WatcherAPI.Spec.DatabaseAccount,
			)
			errorString := fmt.Sprintf(
				condition.InputReadyErrorMessage,
				customErrorString,
			)
			th.ExpectConditionWithDetails(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				errorString,
			)
		})
		It("should have config service unknown", func() {
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionUnknown,
			)
		})
	})
})
