package functional

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	//revive:disable-next-line:dot-imports
	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"
	watcherv1beta1 "github.com/openstack-k8s-operators/watcher-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
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
			Expect(WatcherApplier.Spec.MemcachedInstance).Should(Equal("memcached"))
			Expect(WatcherApplier.Spec.PasswordSelectors).Should(Equal(watcherv1beta1.PasswordSelector{Service: "WatcherPassword"}))
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
			Expect(WatcherApplier.Spec.MemcachedInstance).Should(Equal("memcached"))
			Expect(WatcherApplier.Spec.PasswordSelectors).Should(Equal(watcherv1beta1.PasswordSelector{Service: "WatcherPassword"}))
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
			DeferCleanup(th.DeleteInstance, CreateWatcherApplier(watcherTest.WatcherApplier, GetDefaultWatcherApplierSpec()))
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.WatcherApplier.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
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
					"WatcherPassword":   []byte("service-password"),
					"transport_url":     []byte("url"),
					"database_username": []byte("username"),
					"database_password": []byte("password"),
					"database_hostname": []byte("hostname"),
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
	When("secret, db and memcached are created", func() {
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
		It("should have config service input true", func() {
			th.ExpectCondition(
				watcherTest.WatcherApplier,
				ConditionGetterFunc(WatcherApplierConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
})