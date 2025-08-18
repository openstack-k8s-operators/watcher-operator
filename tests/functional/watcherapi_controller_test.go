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
	MinimalWatcherAPISpec = map[string]interface{}{
		"secret":            "osp-secret",
		"memcachedInstance": "memcached",
	}
)

var _ = Describe("WatcherAPI controller with minimal spec values", func() {
	When("A WatcherAPI instance is created from minimal spec", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateWatcherAPI(watcherTest.WatcherAPI, MinimalWatcherAPISpec))
		})

		It("should have the Spec fields defaulted", func() {
			WatcherAPI := GetWatcherAPI(watcherTest.WatcherAPI)
			Expect(WatcherAPI.Spec.Secret).Should(Equal("osp-secret"))
			Expect(*(WatcherAPI.Spec.MemcachedInstance)).Should(Equal("memcached"))
			Expect(WatcherAPI.Spec.PasswordSelectors).Should(Equal(watcherv1beta1.PasswordSelector{Service: ptr.To("WatcherPassword")}))
			Expect(*(WatcherAPI.Spec.PrometheusSecret)).Should(Equal("metric-storage-prometheus-endpoint"))
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
			Expect(*(WatcherAPI.Spec.MemcachedInstance)).Should(Equal("memcached"))
			Expect(*(WatcherAPI.Spec.PrometheusSecret)).Should(Equal("metric-storage-prometheus-endpoint"))
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

		It("should return false when using isReady method", func() {
			WatcherAPI := GetWatcherAPI(watcherTest.WatcherAPI)
			Expect(WatcherAPI.IsReady()).Should(BeFalse())
		})
	})
	When("the secret is created with all the expected fields and has all the required infra", func() {
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
					watcherTest.WatcherAPI.Namespace,
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
				watcherTest.WatcherAPI.Namespace,
				"watcher",
				mariadbv1.MariaDBDatabaseSpec{
					Name: "watcher",
				},
			)
			mariadb.SimulateMariaDBAccountCompleted(watcherTest.WatcherDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(watcherTest.WatcherDatabaseName)
			DeferCleanup(th.DeleteInstance, CreateWatcherAPI(watcherTest.WatcherAPI, GetDefaultWatcherAPISpec()))
			keystoneAPIName = keystone.CreateKeystoneAPI(watcherTest.WatcherAPI.Namespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPIName)
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
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherAPIStatefulSet)
			th.ExpectCondition(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)

			deployment := th.GetStatefulSet(watcherTest.WatcherAPIStatefulSet)
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
		It("creates the watcher-api service", func() {
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherAPIStatefulSet)
			th.ExpectCondition(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.CreateServiceReadyCondition,
				corev1.ConditionTrue,
			)
			public := th.GetService(watcherTest.WatcherPublicServiceName)
			Expect(public.Labels["service"]).To(Equal("watcher-api"))
			Expect(public.Labels["endpoint"]).To(Equal("public"))
			internal := th.GetService(watcherTest.WatcherInternalServiceName)
			Expect(internal.Labels["service"]).To(Equal("watcher-api"))
			Expect(internal.Labels["endpoint"]).To(Equal("internal"))
		})
		It("created the keystone endpoint for the watcher-api service", func() {
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherAPIStatefulSet)
			keystone.SimulateKeystoneEndpointReady(watcherTest.WatcherKeystoneEndpointName)
			// it registers the endpointURL as the public endpoint and svc
			// for the internal
			keystoneEndpoint := keystone.GetKeystoneEndpoint(watcherTest.WatcherKeystoneEndpointName)
			endpoints := keystoneEndpoint.Spec.Endpoints
			Expect(endpoints).To(HaveKeyWithValue("public", "http://watcher-public."+watcherTest.WatcherAPI.Namespace+".svc:9322"))
			Expect(endpoints).To(HaveKeyWithValue("internal", "http://watcher-internal."+watcherTest.WatcherAPI.Namespace+".svc:9322"))
			th.ExpectCondition(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.KeystoneEndpointReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("should return true when using isReady method", func() {
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherAPIStatefulSet)
			keystone.SimulateKeystoneEndpointReady(watcherTest.WatcherKeystoneEndpointName)

			th.ExpectCondition(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			WatcherAPI := GetWatcherAPI(watcherTest.WatcherAPI)
			Expect(WatcherAPI.IsReady()).Should(BeTrue())
		})

		It("should have the expected config secret content", func() {

			createdSecret := th.GetSecret(watcherTest.WatcherAPIConfigSecret)
			Expect(createdSecret).ShouldNot(BeNil())
			Expect(createdSecret.Data["00-default.conf"]).ShouldNot(BeNil())

			// extract default config data
			configData := createdSecret.Data["00-default.conf"]
			Expect(configData).ShouldNot(BeNil())

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

		It("updates the KeystoneAuthURL if keystone internal endpoint changes", func() {
			newInternalEndpoint := "https://keystone-internal"

			keystone.UpdateKeystoneAPIEndpoint(keystoneAPIName, "internal", newInternalEndpoint)
			logger.Info("Reconfigured")

			th.ExpectCondition(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				confSecret := th.GetSecret(watcherTest.WatcherAPIConfigSecret)
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
	When("prometheus config secret is not created", func() {
		BeforeEach(func() {
			secret := CreateInternalTopLevelSecret()
			DeferCleanup(k8sClient.Delete, ctx, secret)

			DeferCleanup(th.DeleteInstance, CreateWatcherAPI(watcherTest.WatcherAPI, GetDefaultWatcherAPISpec()))
		})

		It("should have input ready false", func() {
			th.ExpectConditionWithDetails(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				watcherv1beta1.WatcherPrometheusSecretErrorMessage,
			)
		})
	})

	When("secret, db and memcached are created, but there is no keystoneapi", func() {
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
	When("WatcherAPI is created with service overrides", func() {
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
			DeferCleanup(th.DeleteInstance, CreateWatcherAPI(watcherTest.WatcherAPI, GetServiceOverrideWatcherAPISpec()))
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(watcherTest.WatcherAPI.Namespace))
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.WatcherAPI.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					watcherTest.WatcherAPI.Namespace,
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
				watcherTest.WatcherAPI.Namespace,
				"watcher",
				mariadbv1.MariaDBDatabaseSpec{
					Name: "watcher",
				},
			)
			mariadb.SimulateMariaDBAccountCompleted(watcherTest.WatcherDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(watcherTest.WatcherDatabaseName)

		})
		It("creates MetalLB service", func() {
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherAPIStatefulSet)
			// simulate that the internal service got a LoadBalancerIP
			// assigned
			th.SimulateLoadBalancerServiceIP(watcherTest.WatcherInternalServiceName)

			// As the public endpoint is not mentioned in the service override
			// a generic Service is created
			public := th.GetService(watcherTest.WatcherPublicServiceName)
			Expect(public.Annotations).NotTo(HaveKey("metallb.universe.tf/address-pool"))
			Expect(public.Annotations).NotTo(HaveKey("metallb.universe.tf/allow-shared-ip"))
			Expect(public.Annotations).NotTo(HaveKey("metallb.universe.tf/loadBalancerIPs"))
			Expect(public.Labels["service"]).To(Equal("watcher-api"))
			Expect(public.Labels["endpoint"]).To(Equal("public"))

			// As the internal endpoint is configure in the service override it
			// creates a Service with MetalLB annotations
			internal := th.GetService(watcherTest.WatcherInternalServiceName)
			Expect(internal.Annotations).To(HaveKeyWithValue("metallb.universe.tf/address-pool", "osp-internalapi"))
			Expect(internal.Annotations).To(HaveKeyWithValue("metallb.universe.tf/allow-shared-ip", "osp-internalapi"))
			Expect(internal.Annotations).To(HaveKeyWithValue("metallb.universe.tf/loadBalancerIPs", "internal-lb-ip-1,internal-lb-ip-2"))
			Expect(internal.Labels["service"]).To(Equal("watcher-api"))
			Expect(internal.Labels["endpoint"]).To(Equal("internal"))

			// simulate the keystone endpoint
			keystone.SimulateKeystoneEndpointReady(watcherTest.WatcherKeystoneEndpointName)
			th.ExpectCondition(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
	When("WatcherAPI is created with service TLS", func() {
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
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(watcherTest.WatcherPublicCertSecret))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(watcherTest.WatcherInternalCertSecret))
			DeferCleanup(
				k8sClient.Delete, ctx, th.CreateSecret(
					types.NamespacedName{Namespace: watcherTest.Instance.Namespace, Name: "combined-ca-bundle"},
					map[string][]byte{
						"internal-ca-bundle.pem": []byte("some-b64-text"),
						"tls-ca-bundle.pem":      []byte("other-b64-text"),
					},
				))
			DeferCleanup(th.DeleteInstance, CreateWatcherAPI(watcherTest.WatcherAPI, GetTLSWatcherAPISpec()))
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(watcherTest.WatcherAPI.Namespace))
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.WatcherAPI.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					watcherTest.WatcherAPI.Namespace,
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
				watcherTest.WatcherAPI.Namespace,
				"watcher",
				mariadbv1.MariaDBDatabaseSpec{
					Name: "watcher",
				},
			)
			mariadb.SimulateMariaDBAccountCompleted(watcherTest.WatcherDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(watcherTest.WatcherDatabaseName)
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherAPIStatefulSet)

		})
		It("should have the TLS Spec fields set", func() {
			WatcherAPI := GetWatcherAPI(watcherTest.WatcherAPI)
			Expect(*WatcherAPI.Spec.TLS.API.Public.SecretName).Should(Equal("cert-watcher-public-svc"))
			Expect(*WatcherAPI.Spec.TLS.API.Internal.SecretName).Should(Equal("cert-watcher-internal-svc"))
			Expect(WatcherAPI.Spec.TLS.CaBundleSecretName).Should(Equal("combined-ca-bundle"))
		})
		It("should have https endpoints", func() {
			keystone.SimulateKeystoneEndpointReady(watcherTest.WatcherKeystoneEndpointName)
			th.ExpectCondition(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.CreateServiceReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
			// it registers the endpointURL as the public endpoint and svc
			// for the internal
			keystoneEndpoint := keystone.GetKeystoneEndpoint(watcherTest.WatcherKeystoneEndpointName)
			endpoints := keystoneEndpoint.Spec.Endpoints
			Expect(endpoints).To(HaveKeyWithValue("public", "https://watcher-public."+watcherTest.WatcherAPI.Namespace+".svc:9322"))
			Expect(endpoints).To(HaveKeyWithValue("internal", "https://watcher-internal."+watcherTest.WatcherAPI.Namespace+".svc:9322"))
		})
	})
	When("WatcherAPI is created with service TLS but invalid cert secret", func() {
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
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(watcherTest.WatcherPublicCertSecret))
			DeferCleanup(
				k8sClient.Delete, ctx, th.CreateSecret(
					watcherTest.WatcherInternalCertSecret,
					map[string][]byte{
						"random-field": []byte("some-b64-text"),
					},
				))
			DeferCleanup(
				k8sClient.Delete, ctx, th.CreateSecret(
					types.NamespacedName{Namespace: watcherTest.Instance.Namespace, Name: "combined-ca-bundle"},
					map[string][]byte{
						"internal-ca-bundle.pem": []byte("some-b64-text"),
						"tls-ca-bundle.pem":      []byte("other-b64-text"),
					},
				))
			DeferCleanup(th.DeleteInstance, CreateWatcherAPI(watcherTest.WatcherAPI, GetTLSWatcherAPISpec()))
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(watcherTest.WatcherAPI.Namespace))
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.WatcherAPI.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					watcherTest.WatcherAPI.Namespace,
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
				watcherTest.WatcherAPI.Namespace,
				"watcher",
				mariadbv1.MariaDBDatabaseSpec{
					Name: "watcher",
				},
			)
			mariadb.SimulateMariaDBAccountCompleted(watcherTest.WatcherDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(watcherTest.WatcherDatabaseName)
		})
		It("fail for invalid TLS input", func() {
			th.ExpectConditionWithDetails(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"TLSInput error occured in TLS sources field not found in Secret: field tls.key not found in Secret "+watcherTest.WatcherAPI.Namespace+"/cert-watcher-internal-svc",
			)
		})
	})
	When("WatcherAPI is created with an invalid CA bundle secret", func() {
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
				k8sClient.Delete, ctx, th.CreateSecret(
					types.NamespacedName{Namespace: watcherTest.Instance.Namespace, Name: "combined-ca-bundle"},
					map[string][]byte{
						"random-field": []byte("some-b64-text"),
					},
				))
			DeferCleanup(th.DeleteInstance, CreateWatcherAPI(watcherTest.WatcherAPI, GetTLSCaWatcherAPISpec()))
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(watcherTest.WatcherAPI.Namespace))
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.WatcherAPI.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					watcherTest.WatcherAPI.Namespace,
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
				watcherTest.WatcherAPI.Namespace,
				"watcher",
				mariadbv1.MariaDBDatabaseSpec{
					Name: "watcher",
				},
			)
			mariadb.SimulateMariaDBAccountCompleted(watcherTest.WatcherDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(watcherTest.WatcherDatabaseName)
		})
		It("fail for invalid TLS input", func() {
			th.ExpectConditionWithDetails(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"TLSInput error occured in TLS sources field not found in Secret: field tls-ca-bundle.pem not found in Secret "+watcherTest.WatcherAPI.Namespace+"/combined-ca-bundle",
			)
		})
	})
	When("WatcherAPI is created with a wrong topologyRef", func() {
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
				k8sClient.Delete, ctx, th.CreateSecret(
					types.NamespacedName{Namespace: watcherTest.Instance.Namespace, Name: "combined-ca-bundle"},
					map[string][]byte{
						"random-field": []byte("some-b64-text"),
					},
				))
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(watcherTest.WatcherAPI.Namespace))
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.WatcherAPI.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					watcherTest.WatcherAPI.Namespace,
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
				watcherTest.WatcherAPI.Namespace,
				"watcher",
				mariadbv1.MariaDBDatabaseSpec{
					Name: "watcher",
				},
			)
			mariadb.SimulateMariaDBAccountCompleted(watcherTest.WatcherDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(watcherTest.WatcherDatabaseName)

			spec := GetDefaultWatcherAPISpec()
			spec["topologyRef"] = map[string]interface{}{"name": "foo"}
			DeferCleanup(th.DeleteInstance, CreateWatcherAPI(watcherTest.WatcherAPI, spec))
		})
		It("points to a non existing topology CR", func() {
			th.ExpectCondition(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})
	})

	When("WatcherAPI is created with topology", func() {
		var topologyRefAPI topologyv1.TopoRef
		var topologyRefAlt topologyv1.TopoRef
		var expectedTopologySpec []corev1.TopologySpreadConstraint
		BeforeEach(func() {
			var topologySpec map[string]interface{}
			// Build the topology Spec
			topologySpec, expectedTopologySpec = GetSampleTopologySpec("watcher-api")
			_ = expectedTopologySpec
			// Create Test Topologies
			_, topologyRefAPI = infra.CreateTopology(
				types.NamespacedName{
					Namespace: namespace,
					Name:      "watcherapi"},
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
			DeferCleanup(
				k8sClient.Delete, ctx, th.CreateSecret(
					types.NamespacedName{Namespace: watcherTest.Instance.Namespace, Name: "combined-ca-bundle"},
					map[string][]byte{
						"random-field": []byte("some-b64-text"),
					},
				))
			spec := GetDefaultWatcherAPISpec()
			spec["topologyRef"] = map[string]interface{}{"name": topologyRefAPI.Name}
			DeferCleanup(th.DeleteInstance, CreateWatcherAPI(watcherTest.WatcherAPI, spec))
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(watcherTest.WatcherAPI.Namespace))
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.WatcherAPI.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					watcherTest.WatcherAPI.Namespace,
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
				watcherTest.WatcherAPI.Namespace,
				"watcher",
				mariadbv1.MariaDBDatabaseSpec{
					Name: "watcher",
				},
			)
			mariadb.SimulateMariaDBAccountCompleted(watcherTest.WatcherDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(watcherTest.WatcherDatabaseName)
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherAPIStatefulSet)

		})
		It("sets lastAppliedTopology field in WatcherAPI topology .Status", func() {

			WatcherAPI := GetWatcherAPI(watcherTest.WatcherAPI)

			Expect(WatcherAPI.Status.LastAppliedTopology).ToNot(BeNil())
			Expect(WatcherAPI.Status.LastAppliedTopology).To(Equal(&topologyRefAPI))

			th.ExpectCondition(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				ss := th.GetStatefulSet(watcherTest.WatcherAPIStatefulSet)
				podTemplate := ss.Spec.Template.Spec
				g.Expect(podTemplate.TopologySpreadConstraints).ToNot(BeNil())
				// No default Pod Antiaffinity is applied
				g.Expect(podTemplate.Affinity).To(BeNil())
			}, timeout, interval).Should(Succeed())

			// Check finalizer
			Eventually(func(g Gomega) {
				tp := infra.GetTopology(types.NamespacedName{
					Name:      topologyRefAPI.Name,
					Namespace: topologyRefAPI.Namespace,
				})
				finalizers := tp.GetFinalizers()
				g.Expect(finalizers).To(ContainElement(
					fmt.Sprintf("openstack.org/watcherapi-%s", watcherTest.WatcherAPI.Name)))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				tpAlt := infra.GetTopology(types.NamespacedName{
					Name:      topologyRefAlt.Name,
					Namespace: topologyRefAlt.Namespace,
				})
				finalizers := tpAlt.GetFinalizers()
				g.Expect(finalizers).ToNot(ContainElement(
					fmt.Sprintf("openstack.org/watcherapi-%s", watcherTest.WatcherAPI.Name)))
			}, timeout, interval).Should(Succeed())
		})
		It("updates lastAppliedTopology in WatcherAPI .Status", func() {
			Eventually(func(g Gomega) {
				WatcherAPI := GetWatcherAPI(watcherTest.WatcherAPI)
				WatcherAPI.Spec.TopologyRef.Name = topologyRefAlt.Name
				g.Expect(k8sClient.Update(ctx, WatcherAPI)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				WatcherAPI := GetWatcherAPI(watcherTest.WatcherAPI)
				g.Expect(WatcherAPI.Status.LastAppliedTopology).ToNot(BeNil())
				g.Expect(WatcherAPI.Status.LastAppliedTopology).To(Equal(&topologyRefAlt))
			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				ss := th.GetStatefulSet(watcherTest.WatcherAPI)
				podTemplate := ss.Spec.Template.Spec
				g.Expect(podTemplate.TopologySpreadConstraints).ToNot(BeNil())
				// No default Pod Antiaffinity is applied
				g.Expect(podTemplate.Affinity).To(BeNil())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				ss := th.GetStatefulSet(watcherTest.WatcherAPI)
				podTemplate := ss.Spec.Template.Spec
				g.Expect(podTemplate.TopologySpreadConstraints).To(Equal(expectedTopologySpec))
			}, timeout, interval).Should(Succeed())

			// Check finalizer is set to topologyRefAlt and is not set to
			// topologyRef
			Eventually(func(g Gomega) {
				tp := infra.GetTopology(types.NamespacedName{
					Name:      topologyRefAPI.Name,
					Namespace: topologyRefAPI.Namespace,
				})
				finalizers := tp.GetFinalizers()
				g.Expect(finalizers).ToNot(ContainElement(
					fmt.Sprintf("openstack.org/watcherapi-%s", watcherTest.WatcherAPI.Name)))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				tpAlt := infra.GetTopology(types.NamespacedName{
					Name:      topologyRefAlt.Name,
					Namespace: topologyRefAlt.Namespace,
				})
				finalizers := tpAlt.GetFinalizers()
				g.Expect(finalizers).To(ContainElement(
					fmt.Sprintf("openstack.org/watcherapi-%s", watcherTest.WatcherAPI.Name)))
			}, timeout, interval).Should(Succeed())
		})
		It("removes topologyRef from WatcherAPI spec", func() {
			Eventually(func(g Gomega) {
				WatcherAPI := GetWatcherAPI(watcherTest.WatcherAPI)
				WatcherAPI.Spec.TopologyRef = nil
				g.Expect(k8sClient.Update(ctx, WatcherAPI)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				WatcherAPI := GetWatcherAPI(watcherTest.WatcherAPI)
				g.Expect(WatcherAPI.Status.LastAppliedTopology).Should(BeNil())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				ss := th.GetStatefulSet(watcherTest.WatcherAPI)
				podTemplate := ss.Spec.Template.Spec
				g.Expect(podTemplate.TopologySpreadConstraints).To(BeNil())
				// Default Pod AntiAffinity is applied
				g.Expect(podTemplate.Affinity).ToNot(BeNil())
			}, timeout, interval).Should(Succeed())

			// Check finalizer is not present anymore
			Eventually(func(g Gomega) {
				tp := infra.GetTopology(types.NamespacedName{
					Name:      topologyRefAPI.Name,
					Namespace: topologyRefAPI.Namespace,
				})
				finalizers := tp.GetFinalizers()
				g.Expect(finalizers).ToNot(ContainElement(
					fmt.Sprintf("openstack.org/watcherapi-%s", watcherTest.WatcherAPI.Name)))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				tpAlt := infra.GetTopology(types.NamespacedName{
					Name:      topologyRefAlt.Name,
					Namespace: topologyRefAlt.Namespace,
				})
				finalizers := tpAlt.GetFinalizers()
				g.Expect(finalizers).ToNot(ContainElement(
					fmt.Sprintf("openstack.org/watcherapi-%s", watcherTest.WatcherAPI.Name)))
			}, timeout, interval).Should(Succeed())
		})
	})

	When("the secret is created with notification_url field in the top level secret", func() {
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
					watcherTest.WatcherAPI.Namespace,
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
				watcherTest.WatcherAPI.Namespace,
				"watcher",
				mariadbv1.MariaDBDatabaseSpec{
					Name: "watcher",
				},
			)
			mariadb.SimulateMariaDBAccountCompleted(watcherTest.WatcherDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(watcherTest.WatcherDatabaseName)
			DeferCleanup(th.DeleteInstance, CreateWatcherAPI(watcherTest.WatcherAPI, GetDefaultWatcherAPISpec()))
			keystoneAPIName = keystone.CreateKeystoneAPI(watcherTest.WatcherAPI.Namespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPIName)
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.WatcherAPI.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherAPIStatefulSet)
			keystone.SimulateKeystoneEndpointReady(watcherTest.WatcherKeystoneEndpointName)

		})
		It("should have input ready", func() {
			th.ExpectCondition(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.InputReadyCondition,
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

		It("should have the expected config secret content", func() {

			createdSecret := th.GetSecret(watcherTest.WatcherAPIConfigSecret)
			Expect(createdSecret).ShouldNot(BeNil())
			Expect(createdSecret.Data["00-default.conf"]).ShouldNot(BeNil())

			// extract default config data
			configData := createdSecret.Data["00-default.conf"]
			Expect(configData).ShouldNot(BeNil())

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

	When("A WatcherAPI instance is created with MTLS memcached", func() {
		BeforeEach(func() {
			// Create the required secret for WatcherAPI
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
			apiMariaDBAccount, apiMariaDBSecret := mariadb.CreateMariaDBAccountAndSecret(
				watcherTest.WatcherDatabaseAccount, mariadbv1.MariaDBAccountSpec{})
			DeferCleanup(k8sClient.Delete, ctx, apiMariaDBAccount)
			DeferCleanup(k8sClient.Delete, ctx, apiMariaDBSecret)

			memcachedSpec := infra.GetDefaultMemcachedSpec()
			// Create Memcached with MTLS auth
			DeferCleanup(infra.DeleteMemcached, infra.CreateMTLSMemcached(watcherTest.WatcherAPI.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMTLSMemcachedReady(watcherTest.MemcachedNamespace)

			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(watcherTest.WatcherAPI.Namespace))

			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					watcherTest.WatcherAPI.Namespace,
					"openstack",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)

			DeferCleanup(th.DeleteInstance, CreateWatcherAPI(watcherTest.WatcherAPI, GetDefaultWatcherAPISpec()))
		})

		It("should have MTLS volumes mounted in StatefulSet", func() {
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherAPIStatefulSet)
			Eventually(func(g Gomega) {
				ss := th.GetStatefulSet(watcherTest.WatcherAPIStatefulSet)

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
					if container.Name == "watcher-api" {
						mtlsVolumeMountFound := false
						for _, volumeMount := range container.VolumeMounts {
							if volumeMount.Name == "cert-memcached-mtls" {
								mtlsVolumeMountFound = true
								break
							}
						}
						g.Expect(mtlsVolumeMountFound).To(BeTrue(), "MTLS volume mount should be present in watcher-api container")
					}
				}
			}, timeout, interval).Should(Succeed())
		})

		It("should have MTLS configuration in config secret", func() {
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherAPIStatefulSet)
			Eventually(func(g Gomega) {
				configSecret := th.GetSecret(watcherTest.WatcherAPIConfigSecret)
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
					watcherTest.WatcherAPI.Namespace,
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
				watcherTest.WatcherAPI.Namespace,
				"watcher",
				mariadbv1.MariaDBDatabaseSpec{
					Name: "watcher",
				},
			)
			mariadb.SimulateMariaDBAccountCompleted(watcherTest.WatcherDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(watcherTest.WatcherDatabaseName)
			DeferCleanup(th.DeleteInstance, CreateWatcherAPI(watcherTest.WatcherAPI, GetDefaultWatcherAPISpec()))
			keystoneAPIName = keystone.CreateKeystoneAPI(watcherTest.WatcherAPI.Namespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPIName)
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.WatcherAPI.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherAPIStatefulSet)
			keystone.SimulateKeystoneEndpointReady(watcherTest.WatcherKeystoneEndpointName)

		})
		It("should have input ready", func() {
			th.ExpectCondition(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.InputReadyCondition,
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

		It("should have the expected config secret content with quorum queue configuration", func() {

			createdSecret := th.GetSecret(watcherTest.WatcherAPIConfigSecret)
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
					watcherTest.WatcherAPI.Namespace,
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
				watcherTest.WatcherAPI.Namespace,
				"watcher",
				mariadbv1.MariaDBDatabaseSpec{
					Name: "watcher",
				},
			)
			mariadb.SimulateMariaDBAccountCompleted(watcherTest.WatcherDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(watcherTest.WatcherDatabaseName)
			DeferCleanup(th.DeleteInstance, CreateWatcherAPI(watcherTest.WatcherAPI, GetDefaultWatcherAPISpec()))
			keystoneAPIName = keystone.CreateKeystoneAPI(watcherTest.WatcherAPI.Namespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPIName)
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.WatcherAPI.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherAPIStatefulSet)
			keystone.SimulateKeystoneEndpointReady(watcherTest.WatcherKeystoneEndpointName)

		})
		It("should have input ready", func() {
			th.ExpectCondition(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.InputReadyCondition,
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

		It("should have the expected config secret content with default quorum queue settings", func() {

			createdSecret := th.GetSecret(watcherTest.WatcherAPIConfigSecret)
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
					watcherTest.WatcherAPI.Namespace,
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
				watcherTest.WatcherAPI.Namespace,
				"watcher",
				mariadbv1.MariaDBDatabaseSpec{
					Name: "watcher",
				},
			)
			mariadb.SimulateMariaDBAccountCompleted(watcherTest.WatcherDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(watcherTest.WatcherDatabaseName)
			DeferCleanup(th.DeleteInstance, CreateWatcherAPI(watcherTest.WatcherAPI, GetDefaultWatcherAPISpec()))
			keystoneAPIName = keystone.CreateKeystoneAPI(watcherTest.WatcherAPI.Namespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPIName)
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.WatcherAPI.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherAPIStatefulSet)
			keystone.SimulateKeystoneEndpointReady(watcherTest.WatcherKeystoneEndpointName)

		})

		It("should reconfigure when quorum queues are disabled", func() {
			// First verify quorum queues are enabled initially
			Eventually(func(g Gomega) {
				createdSecret := th.GetSecret(watcherTest.WatcherAPIConfigSecret)
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
				createdSecret := th.GetSecret(watcherTest.WatcherAPIConfigSecret)
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
