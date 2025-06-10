package functional

import (
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	//revive:disable-next-line:dot-imports

	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	rabbitmqv1 "github.com/openstack-k8s-operators/infra-operator/apis/rabbitmq/v1beta1"
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	keystonev1beta1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
	watcherv1beta1 "github.com/openstack-k8s-operators/watcher-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
			Expect(*(Watcher.Spec.DatabaseInstance)).Should(Equal("openstack"))
			Expect(Watcher.Spec.DatabaseAccount).Should(Equal("watcher"))
			Expect(Watcher.Spec.Secret).Should(Equal("osp-secret"))
			Expect(Watcher.Spec.PasswordSelectors).Should(Equal(watcherv1beta1.PasswordSelector{Service: "WatcherPassword"}))
			Expect(*(Watcher.Spec.RabbitMqClusterName)).Should(Equal("rabbitmq"))
			Expect(Watcher.Spec.ServiceUser).Should(Equal("watcher"))
			Expect(Watcher.Spec.PreserveJobs).Should(BeFalse())
			Expect(Watcher.Spec.APIServiceTemplate.TLS.CaBundleSecretName).Should(Equal(""))
			Expect(Watcher.Spec.CustomServiceConfig).Should(Equal(""))
			Expect(Watcher.Spec.PrometheusSecret).Should(Equal("metric-storage-prometheus-endpoint"))
			Expect(Watcher.Spec.APIServiceTemplate.CustomServiceConfig).Should(Equal(""))
			Expect(*(Watcher.Spec.DBPurge.Schedule)).Should(Equal("0 1 * * *"))
			Expect(*(Watcher.Spec.DBPurge.PurgeAge)).Should(Equal(90))

		})

		It("should have the Status fields initialized", func() {
			Watcher := GetWatcher(watcherTest.Instance)
			Expect(Watcher.Status.ObservedGeneration).To(Equal(int64(0)))
			Expect(Watcher.Status.ServiceID).Should(Equal(""))
			Expect(Watcher.Status.Hash).Should(BeEmpty())
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
			Expect(*(Watcher.Spec.DatabaseInstance)).Should(Equal("openstack"))
			Expect(Watcher.Spec.DatabaseAccount).Should(Equal("watcher"))
			Expect(Watcher.Spec.ServiceUser).Should(Equal("watcher"))
			Expect(Watcher.Spec.Secret).Should(Equal("test-osp-secret"))
			Expect(*(Watcher.Spec.RabbitMqClusterName)).Should(Equal("rabbitmq"))
			Expect(Watcher.Spec.PreserveJobs).Should(BeFalse())

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

			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				condition.CronJobReadyCondition,
				corev1.ConditionUnknown,
			)

			Watcher := GetWatcher(watcherTest.Instance)
			Expect(Watcher.IsReady()).Should(BeFalse())

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

	When("Watcher is created with default Spec", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateWatcher(watcherTest.Instance, GetDefaultWatcherSpec()))
			DeferCleanup(k8sClient.Delete, ctx, CreateWatcherMessageBusSecret(watcherTest.Instance.Namespace, "rabbitmq-secret"))
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.Watcher.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					watcherTest.Instance.Namespace,
					*GetWatcher(watcherTest.Instance).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(watcherTest.WatcherAPI.Namespace))
			DeferCleanup(
				k8sClient.Delete, ctx, th.CreateSecret(
					types.NamespacedName{Namespace: watcherTest.Instance.Namespace, Name: "metric-storage-prometheus-endpoint"},
					map[string][]byte{
						"host": []byte("prometheus.example.com"),
						"port": []byte("9090"),
					},
				))
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

		It("Should create the watcher successfully with default spec values", func() {
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

			// Simulate dbsync success
			th.SimulateJobSuccess(watcherTest.WatcherDBSync)
			// We validate the full Watcher CR readiness status here
			// DB Ready

			// Simulate WatcherAPI deployment
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherAPIStatefulSet)

			// Simulate KeystoneEndpoint success
			keystone.SimulateKeystoneEndpointReady(watcherTest.WatcherKeystoneEndpointName)

			// Simulate WatcherApplier deployment
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherApplierStatefulSet)

			// Simulate WatcherDecisionEngine deployment
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherDecisionEngineStatefulSet)

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

			// DBSync execution
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)

			// Get WatcherAPI Ready condition
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				watcherv1beta1.WatcherAPIReadyCondition,
				corev1.ConditionTrue,
			)

			// WatcherApplier in Ready condition
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				watcherv1beta1.WatcherApplierReadyCondition,
				corev1.ConditionTrue,
			)

			// Get WatcherDecisionEngine Ready condition
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				watcherv1beta1.WatcherDecisionEngineReadyCondition,
				corev1.ConditionTrue,
			)

			// Get CronJobReadyCondition Ready condition
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				condition.CronJobReadyCondition,
				corev1.ConditionTrue,
			)

			// Global status Ready
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			Watcher := GetWatcher(watcherTest.Instance)
			Expect(Watcher.IsReady()).Should(BeTrue())

			// assert that the KeystoneService for watcher is created
			ksrvList := &keystonev1beta1.KeystoneServiceList{}
			listOpts := &client.ListOptions{
				FieldSelector: fields.OneTermEqualSelector("metadata.name", "watcher"),
				Namespace:     watcherTest.Instance.Namespace,
			}
			_ = th.K8sClient.List(ctx, ksrvList, listOpts)
			Expect(ksrvList.Items).ToNot(BeEmpty())
			Expect(ksrvList.Items[0].Status.Conditions).ToNot(BeNil())

			// status.hash['dbsync'] should be populated when dbsync is successful
			Expect(Watcher.Status.Hash[watcherv1beta1.DbSyncHash]).ShouldNot(BeNil())

			// assert that the top level secret is created
			createdSecret := th.GetSecret(watcherTest.Watcher)
			Expect(createdSecret).ShouldNot(BeNil())
			Expect(createdSecret.Data["WatcherPassword"]).To(Equal([]byte("password")))
			Expect(createdSecret.Data["transport_url"]).To(Equal([]byte("rabbit://rabbitmq-secret/fake")))
			Expect(createdSecret.Data["database_account"]).To(Equal([]byte("watcher")))
			Expect(createdSecret.Data["01-global-custom.conf"]).To(Equal([]byte("")))

			// Check WatcherAPI is created
			WatcherAPI := GetWatcherAPI(watcherTest.WatcherAPI)
			Expect(WatcherAPI.Spec.ContainerImage).To(Equal(watcherv1beta1.WatcherAPIContainerImage))
			Expect(WatcherAPI.Spec.Secret).To(Equal("watcher"))
			Expect(WatcherAPI.Spec.ServiceAccount).To(Equal("watcher-watcher"))
			Expect(int(*WatcherAPI.Spec.Replicas)).To(Equal(1))
			Expect(WatcherAPI.Spec.NodeSelector).To(BeNil())
			Expect(WatcherAPI.Spec.CustomServiceConfig).To(Equal(""))
			Expect(WatcherAPI.Spec.PrometheusSecret).Should(Equal("metric-storage-prometheus-endpoint"))

			// Assert that the watcher statefulset is created
			deployment := th.GetStatefulSet(watcherTest.WatcherAPIStatefulSet)
			Expect(deployment.Spec.Template.Spec.ServiceAccountName).To(Equal("watcher-watcher"))
			Expect(int(*deployment.Spec.Replicas)).To(Equal(1))
			Expect(deployment.Spec.Template.Spec.Volumes).To(HaveLen(3))
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(2))
			Expect(deployment.Spec.Selector.MatchLabels).To(Equal(map[string]string{"service": "watcher-api"}))

			// Check if WatcherApplier is created
			WatcherApplier := GetWatcherApplier(watcherTest.WatcherApplier)
			Expect(WatcherApplier.Spec.ContainerImage).To(Equal(watcherv1beta1.WatcherApplierContainerImage))
			Expect(WatcherApplier.Spec.Secret).To(Equal("watcher"))
			Expect(WatcherApplier.Spec.ServiceAccount).To(Equal("watcher-watcher"))
			Expect(int(*WatcherApplier.Spec.Replicas)).To(Equal(1))
			Expect(WatcherApplier.Spec.NodeSelector).To(BeNil())
			Expect(WatcherApplier.Spec.CustomServiceConfig).To(Equal(""))

			// Assert that the watcher applier statefulset is created
			applierDeploy := th.GetStatefulSet(watcherTest.WatcherApplierStatefulSet)
			Expect(applierDeploy.Spec.Template.Spec.ServiceAccountName).To(Equal("watcher-watcher"))
			Expect(int(*applierDeploy.Spec.Replicas)).To(Equal(1))
			Expect(applierDeploy.Spec.Template.Spec.Volumes).To(HaveLen(3))
			Expect(applierDeploy.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(applierDeploy.Spec.Selector.MatchLabels).To(Equal(map[string]string{"service": "watcher-applier"}))

			prometheusSecret := th.GetSecret(types.NamespacedName{Namespace: watcherTest.Instance.Namespace, Name: "metric-storage-prometheus-endpoint"})
			Expect(prometheusSecret.Finalizers).To(ContainElement("openstack.org/watcher"))
			// Check WatcherDecisionEngine is created
			WatcherDecisionEngine := GetWatcherDecisionEngine(watcherTest.WatcherDecisionEngine)
			Expect(WatcherDecisionEngine.Spec.ContainerImage).To(Equal(watcherv1beta1.WatcherDecisionEngineContainerImage))
			Expect(WatcherDecisionEngine.Spec.Secret).To(Equal("watcher"))
			Expect(WatcherDecisionEngine.Spec.ServiceAccount).To(Equal("watcher-watcher"))
			Expect(int(*WatcherDecisionEngine.Spec.Replicas)).To(Equal(1))
			Expect(WatcherDecisionEngine.Spec.NodeSelector).To(BeNil())
			Expect(WatcherDecisionEngine.Spec.CustomServiceConfig).To(Equal(""))
			Expect(WatcherDecisionEngine.Spec.PrometheusSecret).Should(Equal("metric-storage-prometheus-endpoint"))

			// Assert that the Watcher DecisionEngine statefulset is created
			decisionengineStatefulSet := th.GetStatefulSet(watcherTest.WatcherDecisionEngineStatefulSet)
			Expect(decisionengineStatefulSet.Spec.Template.Spec.ServiceAccountName).To(Equal("watcher-watcher"))
			Expect(int(*decisionengineStatefulSet.Spec.Replicas)).To(Equal(1))
			Expect(decisionengineStatefulSet.Spec.Template.Spec.Volumes).To(HaveLen(3))
			Expect(decisionengineStatefulSet.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(decisionengineStatefulSet.Spec.Selector.MatchLabels).To(Equal(map[string]string{"service": "watcher-decision-engine"}))

			// The CronJob for DB Purge is created properly
			// Check the scripts secret is created
			scriptsSecret := th.GetSecret(
				types.NamespacedName{
					Name:      watcherTest.Instance.Name + "-scripts",
					Namespace: watcherTest.Instance.Namespace,
				},
			)

			Expect(scriptsSecret).ShouldNot(BeNil())
			Expect(scriptsSecret.Data).Should(HaveKey("dbpurge.sh"))
			scriptData := string(scriptsSecret.Data["dbpurge.sh"])
			Expect(scriptData).To(ContainSubstring("watcher-db-manage --config-dir /etc/watcher/watcher.conf.d/ --debug purge"))
			cron := GetCronJob(types.NamespacedName{Namespace: watcherTest.Instance.Namespace,
				Name: watcherTest.Instance.Name + "-db-purge"})

			jobEnv := cron.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Env
			Expect(cron.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Image).To(
				Equal(Watcher.Spec.APIContainerImageURL))
			Expect(cron.Spec.Schedule).To(Equal(*Watcher.Spec.DBPurge.Schedule))
			Expect(cron.Labels["service"]).To(Equal("watcher"))
			Expect(GetEnvVarValue(jobEnv, "PURGE_AGE", "")).To(
				Equal(fmt.Sprintf("%d", *Watcher.Spec.DBPurge.PurgeAge)))
		})
		It("Should expose the watcher public service without TLS", func() {
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

			// Simulate dbsync success
			th.SimulateJobSuccess(watcherTest.WatcherDBSync)
			// We validate the full Watcher CR readiness status here
			// DB Ready

			// Simulate WatcherAPI deployment
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherAPIStatefulSet)

			// Simulate KeystoneEndpoint success
			keystone.SimulateKeystoneEndpointReady(watcherTest.WatcherKeystoneEndpointName)

			// Simulate WatcherApplier deployment
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherApplierStatefulSet)

			// Simulate WatcherDecisionEngine deployment
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherDecisionEngineStatefulSet)

			public := th.GetService(watcherTest.WatcherPublicServiceName)
			Expect(public.Labels["service"]).To(Equal("watcher-api"))
			Expect(public.Labels["endpoint"]).To(Equal("public"))
			internal := th.GetService(watcherTest.WatcherInternalServiceName)
			Expect(internal.Labels["service"]).To(Equal("watcher-api"))
			Expect(internal.Labels["endpoint"]).To(Equal("internal"))
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
					*GetWatcher(watcherTest.Instance).Spec.DatabaseInstance,
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

	When("Watcher is created with container images defined in CR and env variables contains fake values", func() {
		BeforeEach(func() {
			// Set environment variables
			os.Setenv("RELATED_IMAGE_WATCHER_API_IMAGE_URL_DEFAULT", "watcher-api-custom-image-env")
			os.Setenv("RELATED_IMAGE_WATCHER_DECISION_ENGINE_IMAGE_URL_DEFAULT", "watcher-decision-engine-custom-image-env")
			os.Setenv("RELATED_IMAGE_WATCHER_APPLIER_IMAGE_URL_DEFAULT", "watcher-applier-custom-image-env")
			DeferCleanup(th.DeleteInstance, CreateWatcher(watcherTest.Instance, MinimalWatcherContainerSpec))
		})

		It("It should have the fields coming from the spec", func() {
			Watcher := GetWatcher(watcherTest.Instance)
			Expect(Watcher.Spec.APIContainerImageURL).To(Equal("watcher-api-custom-image"))
			Expect(Watcher.Spec.DecisionEngineContainerImageURL).To(Equal("watcher-decision-engine-custom-image"))
			Expect(Watcher.Spec.ApplierContainerImageURL).To(Equal("watcher-applier-custom-image"))
		})
	})

	When("Watcher is created with no container images defined in CR and env variables contains fake value", func() {
		BeforeEach(func() {
			os.Setenv("RELATED_IMAGE_WATCHER_API_IMAGE_URL_DEFAULT", "watcher-api-custom-image-env")
			os.Setenv("RELATED_IMAGE_WATCHER_DECISION_ENGINE_IMAGE_URL_DEFAULT", "watcher-decision-engine-custom-image-env")
			os.Setenv("RELATED_IMAGE_WATCHER_APPLIER_IMAGE_URL_DEFAULT", "watcher-applier-custom-image-env")
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

	When("Watcher rejects when empty databaseinstance is used", func() {
		It("should raise an error for empty databaseInstance", func() {
			spec := GetDefaultWatcherAPISpec()
			spec["databaseInstance"] = ""

			raw := map[string]interface{}{
				"apiVersion": "watcher.openstack.org/v1beta1",
				"kind":       "watcher",
				"metadata": map[string]interface{}{
					"name":      watcherName.Name,
					"namespace": watcherName.Namespace,
				},
				"spec": spec,
			}

			unstructuredObj := &unstructured.Unstructured{Object: raw}
			_, err := controllerutil.CreateOrPatch(
				th.Ctx, th.K8sClient, unstructuredObj, func() error { return nil })
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(
				ContainSubstring(
					"admission webhook \"vwatcher.kb.io\" denied the request: " +
						"Watcher.watcher.openstack.org \"watcher\" is invalid: " +
						"spec.databaseInstance: Invalid value: \"\": " +
						"databaseInstance field should not be empty"),
			)

		})
	})

	When("wrong topologies are used in Watcher spec", func() {
		It("should raise an error of wrong topology namespace", func() {

			spec := GetDefaultWatcherAPISpec()
			spec["topologyRef"] = map[string]interface{}{"name": "foo", "namespace": "bar"}

			raw := map[string]interface{}{
				"apiVersion": "watcher.openstack.org/v1beta1",
				"kind":       "watcher",
				"metadata": map[string]interface{}{
					"name":      watcherName.Name,
					"namespace": watcherName.Namespace,
				},
				"spec": spec,
			}

			unstructuredObj := &unstructured.Unstructured{Object: raw}
			_, err := controllerutil.CreateOrPatch(
				th.Ctx, th.K8sClient, unstructuredObj, func() error { return nil })
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(
				ContainSubstring(
					"admission webhook \"vwatcher.kb.io\" denied the request: " +
						"Watcher.watcher.openstack.org \"watcher\" is invalid: " +
						"spec.topologyRef.namespace: Invalid value: \"namespace\": " +
						"Customizing namespace field is not supported"),
			)

		})
	})

	When("Watcher is created with empty RabbitMqClusterName", func() {
		It("should raise an error for empty RabbitMqClusterName", func() {
			spec := GetDefaultWatcherAPISpec()
			spec["rabbitMqClusterName"] = ""

			raw := map[string]interface{}{
				"apiVersion": "watcher.openstack.org/v1beta1",
				"kind":       "watcher",
				"metadata": map[string]interface{}{
					"name":      watcherName.Name,
					"namespace": watcherName.Namespace,
				},
				"spec": spec,
			}

			unstructuredObj := &unstructured.Unstructured{Object: raw}
			_, err := controllerutil.CreateOrPatch(
				th.Ctx, th.K8sClient, unstructuredObj, func() error { return nil })
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(
				ContainSubstring(
					"admission webhook \"vwatcher.kb.io\" denied the request: " +
						"Watcher.watcher.openstack.org \"watcher\" is invalid: " +
						"spec.rabbitMqClusterName: Invalid value: \"\": " +
						"rabbitMqClusterName field should not be empty"),
			)
		})
	})

	When("Watcher with non-default values are created", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateWatcher(watcherTest.Instance, GetNonDefaultWatcherSpec()))
			DeferCleanup(k8sClient.Delete, ctx, CreateWatcherMessageBusSecret(watcherTest.Instance.Namespace, "rabbitmq-secret"))
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.Watcher.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					watcherTest.Instance.Namespace,
					*GetWatcher(watcherTest.Instance).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(watcherTest.WatcherAPI.Namespace))
			DeferCleanup(
				k8sClient.Delete, ctx, th.CreateSecret(
					types.NamespacedName{Namespace: watcherTest.Instance.Namespace, Name: "combined-ca-bundle"},
					map[string][]byte{
						"internal-ca-bundle.pem": []byte("some-b64-text"),
						"tls-ca-bundle.pem":      []byte("other-b64-text"),
					},
				))
			DeferCleanup(
				k8sClient.Delete, ctx, th.CreateSecret(
					types.NamespacedName{Namespace: watcherTest.Instance.Namespace, Name: "custom-prometheus-config"},
					map[string][]byte{
						"host":      []byte("customprometheus.example.com"),
						"port":      []byte("9092"),
						"ca_secret": []byte("combined-ca-bundle"),
						"ca_key":    []byte("internal-ca-bundle.pem"),
					},
				))
		})

		It("should have the Spec fields with the expected values", func() {
			Watcher := GetWatcher(watcherTest.Instance)
			Expect(*(Watcher.Spec.DatabaseInstance)).Should(Equal("fakeopenstack"))
			Expect(Watcher.Spec.DatabaseAccount).Should(Equal("watcher"))
			Expect(Watcher.Spec.ServiceUser).Should(Equal("fakeuser"))
			Expect(Watcher.Spec.Secret).Should(Equal("test-osp-secret"))
			Expect(Watcher.Spec.PreserveJobs).Should(BeTrue())
			Expect(*(Watcher.Spec.RabbitMqClusterName)).Should(Equal("rabbitmq"))
			Expect(Watcher.Spec.APIServiceTemplate.TLS.CaBundleSecretName).Should(Equal("combined-ca-bundle"))
			Expect(Watcher.Spec.CustomServiceConfig).Should(Equal("# Global config"))
			Expect(Watcher.Spec.PrometheusSecret).Should(Equal("custom-prometheus-config"))
			Expect(Watcher.Spec.APIServiceTemplate.CustomServiceConfig).Should(Equal("# Service config"))
			Expect(*(Watcher.Spec.DBPurge.Schedule)).Should(Equal("1 2 * * *"))
			Expect(*(Watcher.Spec.DBPurge.PurgeAge)).Should(Equal(1))
		})

		It("Should create watcher service with custom values", func() {
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

			// Simulate dbsync success
			th.SimulateJobSuccess(watcherTest.WatcherDBSync)

			// Simulate WatcherAPI deployment
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherAPIStatefulSet)

			// Simulate KeystoneEndpoint success
			keystone.SimulateKeystoneEndpointReady(watcherTest.WatcherKeystoneEndpointName)

			// Simulate WatcherApplier deployment
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherApplierStatefulSet)

			// Simulate WatcherDecisionEngine deployment
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherDecisionEngineStatefulSet)

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

			// DBSync execution
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)

			// Get WatcherAPI Ready condition
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				watcherv1beta1.WatcherAPIReadyCondition,
				corev1.ConditionTrue,
			)

			// WatcherApplier in Ready condition
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				watcherv1beta1.WatcherApplierReadyCondition,
				corev1.ConditionTrue,
			)

			// WatcherDecisionEngine Ready condition
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				watcherv1beta1.WatcherDecisionEngineReadyCondition,
				corev1.ConditionTrue,
			)

			// Global status Ready
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			// assert that the MariaDBDatabase is created in non-default Database
			mariadbList := &mariadbv1.MariaDBDatabaseList{}
			listOpts := &client.ListOptions{
				FieldSelector: fields.OneTermEqualSelector("metadata.name", "watcher"),
				Namespace:     watcherTest.Instance.Namespace,
			}
			_ = th.K8sClient.List(ctx, mariadbList, listOpts)
			// Check custom ServiceUser
			Expect(mariadbList.Items[0].Labels["dbName"]).To(Equal("fakeopenstack"))

			// assert that the KeystoneService for watcher is created
			ksrvList := &keystonev1beta1.KeystoneServiceList{}
			listOpts = &client.ListOptions{
				FieldSelector: fields.OneTermEqualSelector("metadata.name", "watcher"),
				Namespace:     watcherTest.Instance.Namespace,
			}
			_ = th.K8sClient.List(ctx, ksrvList, listOpts)
			// Check custom ServiceUser
			Expect(ksrvList.Items[0].Spec.ServiceUser).To(Equal("fakeuser"))

			// status.hash['dbsync'] should be populated when dbsync is successful
			Watcher := GetWatcher(watcherTest.Instance)
			Expect(Watcher.Status.Hash[watcherv1beta1.DbSyncHash]).ShouldNot(BeNil())

			// assert that the top level secret is created with proper content
			createdSecret := th.GetSecret(watcherTest.Watcher)
			Expect(createdSecret).ShouldNot(BeNil())
			Expect(createdSecret.Data["WatcherPassword"]).To(Equal([]byte("password")))
			Expect(createdSecret.Data["transport_url"]).To(Equal([]byte("rabbit://rabbitmq-secret/fake")))
			Expect(createdSecret.Data["01-global-custom.conf"]).To(Equal([]byte("# Global config")))

			// Check WatcherAPI is created with non-default values
			watcherAPI := &watcherv1beta1.WatcherAPI{}
			Expect(k8sClient.Get(ctx,
				types.NamespacedName{Namespace: watcherTest.Instance.Namespace, Name: watcherTest.Instance.Name + "-api"},
				watcherAPI)).Should(Succeed())

			// Check the config-data volume of watcherapi has expected info
			apiConfigSecret := th.GetSecret(
				types.NamespacedName{
					Name:      watcherTest.Instance.Name + "-api-config-data",
					Namespace: watcherTest.Instance.Namespace,
				},
			)
			Expect(apiConfigSecret).ShouldNot(BeNil())
			Expect(apiConfigSecret.Data["my.cnf"]).To(Equal([]byte("[client]\nssl=0")))

			WatcherAPI := GetWatcherAPI(watcherTest.WatcherAPI)
			//Expect(WatcherAPI.Spec.Replicas).To(Equal(int(1)))
			Expect(WatcherAPI.Spec.ContainerImage).To(Equal("fake-API-Container-URL"))
			Expect(WatcherAPI.Spec.Secret).To(Equal("watcher"))
			Expect(WatcherAPI.Spec.ServiceAccount).To(Equal("watcher-watcher"))
			Expect(int(*WatcherAPI.Spec.Replicas)).To(Equal(2))
			Expect(*WatcherAPI.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
			Expect(WatcherAPI.Spec.TLS.CaBundleSecretName).Should(Equal("combined-ca-bundle"))
			Expect(WatcherAPI.Spec.CustomServiceConfig).Should(Equal("# Service config"))
			Expect(WatcherAPI.Spec.PrometheusSecret).Should(Equal("custom-prometheus-config"))

			// Assert that the watcher deployment is created
			deployment := th.GetStatefulSet(watcherTest.WatcherAPIStatefulSet)
			Expect(deployment.Spec.Template.Spec.ServiceAccountName).To(Equal("watcher-watcher"))
			Expect(int(*deployment.Spec.Replicas)).To(Equal(2))
			Expect(deployment.Spec.Template.Spec.Volumes).To(HaveLen(5))
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(2))
			Expect(deployment.Spec.Selector.MatchLabels).To(Equal(map[string]string{"service": "watcher-api"}))

			// Assert that the required custom configuration is applied in the config secret
			// assert that the top level secret is created with proper content
			createdConfigSecret := th.GetSecret(types.NamespacedName{Namespace: watcherTest.Instance.Namespace, Name: watcherTest.Instance.Name + "-api-config-data"})
			Expect(createdConfigSecret).ShouldNot(BeNil())
			Expect(createdConfigSecret.Data["01-global-custom.conf"]).Should(Equal([]byte("# Global config")))
			Expect(createdConfigSecret.Data["02-service-custom.conf"]).Should(Equal([]byte("# Service config")))

			// Check Watcher Applier
			watcherApplier := &watcherv1beta1.WatcherApplier{}
			Expect(k8sClient.Get(ctx,
				types.NamespacedName{Namespace: watcherTest.Instance.Namespace, Name: watcherTest.Instance.Name + "-applier"},
				watcherApplier)).Should(Succeed())

			// Check the config-data volume of watcherapplier has expected info
			applierConfigSecret := th.GetSecret(
				types.NamespacedName{
					Name:      watcherTest.Instance.Name + "-applier-config-data",
					Namespace: watcherTest.Instance.Namespace,
				},
			)
			Expect(applierConfigSecret).ShouldNot(BeNil())
			Expect(applierConfigSecret.Data["my.cnf"]).To(Equal([]byte("[client]\nssl=0")))

			WatcherApplier := GetWatcherApplier(watcherTest.WatcherApplier)
			Expect(WatcherApplier.Spec.ContainerImage).To(Equal("fake-Applier-Container-URL"))
			Expect(WatcherApplier.Spec.Secret).To(Equal("watcher"))
			Expect(WatcherApplier.Spec.ServiceAccount).To(Equal("watcher-watcher"))
			Expect(int(*WatcherApplier.Spec.Replicas)).To(Equal(1))
			Expect(*WatcherApplier.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
			Expect(WatcherApplier.Spec.TLS.CaBundleSecretName).Should(Equal("combined-ca-bundle"))
			Expect(WatcherApplier.Spec.CustomServiceConfig).Should(Equal("# Service config Applier"))

			// Assert that the watcher applier deployment is created
			applierDeploy := th.GetStatefulSet(watcherTest.WatcherApplierStatefulSet)
			Expect(applierDeploy.Spec.Template.Spec.ServiceAccountName).To(Equal("watcher-watcher"))
			Expect(int(*applierDeploy.Spec.Replicas)).To(Equal(1))
			Expect(applierDeploy.Spec.Template.Spec.Volumes).To(HaveLen(4))
			Expect(applierDeploy.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(applierDeploy.Spec.Selector.MatchLabels).To(Equal(map[string]string{"service": "watcher-applier"}))

			// Assert that the required custom configuration is applied in the config secret
			// assert that the top level secret is created with proper content
			createdConfigSecret = th.GetSecret(types.NamespacedName{Namespace: watcherTest.Instance.Namespace, Name: watcherTest.Instance.Name + "-applier-config-data"})
			Expect(createdConfigSecret).ShouldNot(BeNil())
			Expect(createdConfigSecret.Data["01-global-custom.conf"]).Should(Equal([]byte("# Global config")))
			Expect(createdConfigSecret.Data["02-service-custom.conf"]).Should(Equal([]byte("# Service config Applier")))

			// Check WatcherDecisionEngine
			watcherDecisionEngine := &watcherv1beta1.WatcherDecisionEngine{}
			Expect(k8sClient.Get(ctx,
				types.NamespacedName{Namespace: watcherTest.Instance.Namespace, Name: watcherTest.Instance.Name + "-decision-engine"},
				watcherDecisionEngine)).Should(Succeed())

			// Check the config-data volume of watcherDecisionEngine has expected info
			decisionengineConfigSecret := th.GetSecret(
				types.NamespacedName{
					Name:      watcherTest.Instance.Name + "-decision-engine-config-data",
					Namespace: watcherTest.Instance.Namespace,
				},
			)
			Expect(decisionengineConfigSecret).ShouldNot(BeNil())
			Expect(decisionengineConfigSecret.Data["my.cnf"]).To(Equal([]byte("[client]\nssl=0")))

			WatcherDecisionEngine := GetWatcherDecisionEngine(watcherTest.WatcherDecisionEngine)
			Expect(WatcherDecisionEngine.Spec.ContainerImage).To(Equal("fake-DecisionEngine-Container-URL"))
			Expect(WatcherDecisionEngine.Spec.Secret).To(Equal("watcher"))
			Expect(WatcherDecisionEngine.Spec.ServiceAccount).To(Equal("watcher-watcher"))
			Expect(int(*WatcherDecisionEngine.Spec.Replicas)).To(Equal(1))
			Expect(*WatcherDecisionEngine.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
			Expect(WatcherDecisionEngine.Spec.TLS.CaBundleSecretName).Should(Equal("combined-ca-bundle"))
			Expect(WatcherDecisionEngine.Spec.CustomServiceConfig).Should(Equal("# Service config DecisionEngine"))
			Expect(WatcherDecisionEngine.Spec.PrometheusSecret).Should(Equal("custom-prometheus-config"))

			// Assert the DecisionEngine StatefulSet is created
			decisionEngineStatefulSet := th.GetStatefulSet(watcherTest.WatcherDecisionEngineStatefulSet)
			Expect(decisionEngineStatefulSet.Spec.Template.Spec.ServiceAccountName).To(Equal("watcher-watcher"))
			Expect(int(*decisionEngineStatefulSet.Spec.Replicas)).To(Equal(1))
			Expect(decisionEngineStatefulSet.Spec.Template.Spec.Volumes).To(HaveLen(5))
			Expect(decisionEngineStatefulSet.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(decisionEngineStatefulSet.Spec.Selector.MatchLabels).To(Equal(map[string]string{"service": "watcher-decision-engine"}))

			// Assert that the required custom configuration is applied in the config secret
			// assert that the top level secret is created with proper content
			createdConfigSecret = th.GetSecret(types.NamespacedName{Namespace: watcherTest.Instance.Namespace, Name: watcherTest.Instance.Name + "-decision-engine-config-data"})
			Expect(createdConfigSecret).ShouldNot(BeNil())
			Expect(createdConfigSecret.Data["01-global-custom.conf"]).Should(Equal([]byte("# Global config")))
			Expect(createdConfigSecret.Data["02-service-custom.conf"]).Should(Equal([]byte("# Service config DecisionEngine")))

			// The CronJob for DB Purge is created properly
			// Check the scripts secret is created
			scriptsSecret := th.GetSecret(
				types.NamespacedName{
					Name:      watcherTest.Instance.Name + "-scripts",
					Namespace: watcherTest.Instance.Namespace,
				},
			)

			Expect(scriptsSecret).ShouldNot(BeNil())
			Expect(scriptsSecret.Data).Should(HaveKey("dbpurge.sh"))
			scriptData := string(scriptsSecret.Data["dbpurge.sh"])
			Expect(scriptData).To(ContainSubstring("watcher-db-manage --config-dir /etc/watcher/watcher.conf.d/ --debug purge"))
			cron := GetCronJob(types.NamespacedName{Namespace: watcherTest.Instance.Namespace,
				Name: watcherTest.Instance.Name + "-db-purge"})

			jobEnv := cron.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Env
			Expect(cron.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Image).To(
				Equal(Watcher.Spec.APIContainerImageURL))
			Expect(cron.Spec.Schedule).To(Equal("1 2 * * *"))
			Expect(cron.Labels["service"]).To(Equal("watcher"))
			Expect(GetEnvVarValue(jobEnv, "PURGE_AGE", "")).To(
				Equal(fmt.Sprintf("%d", 1)))
		})
	})

	When("The prometheus secret does not exist", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateWatcher(watcherTest.Instance, GetNonDefaultWatcherSpec()))
			DeferCleanup(k8sClient.Delete, ctx, CreateWatcherMessageBusSecret(watcherTest.Instance.Namespace, "rabbitmq-secret"))
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.Watcher.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					watcherTest.Instance.Namespace,
					*GetWatcher(watcherTest.Instance).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			DeferCleanup(
				k8sClient.Delete, ctx, th.CreateSecret(
					types.NamespacedName{Namespace: watcherTest.Instance.Namespace, Name: SecretName},
					map[string][]byte{
						"WatcherPassword": []byte("password"),
					},
				))
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(watcherTest.WatcherAPI.Namespace))
			mariadb.SimulateMariaDBAccountCompleted(watcherTest.WatcherDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(watcherTest.WatcherDatabaseName)
			infra.SimulateTransportURLReady(watcherTest.WatcherTransportURL)

		})

		It("Should set Input Ready to False", func() {
			// Input Ready (secrets)
			th.ExpectConditionWithDetails(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"Error with prometheus config secret",
			)
		})

	})
	When("A Watcher instance with TLSe is created", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateWatcher(watcherTest.Instance, GetTLSeWatcherSpec()))
			DeferCleanup(k8sClient.Delete, ctx, CreateWatcherMessageBusSecret(watcherTest.Instance.Namespace, "rabbitmq-secret"))
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.Watcher.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					watcherTest.Instance.Namespace,
					*GetWatcher(watcherTest.Instance).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(watcherTest.Watcher.Namespace))
			DeferCleanup(
				k8sClient.Delete, ctx, th.CreateSecret(
					types.NamespacedName{Namespace: watcherTest.Instance.Namespace, Name: "metric-storage-prometheus-endpoint"},
					map[string][]byte{
						"host": []byte("prometheus.example.com"),
						"port": []byte("9090"),
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
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(watcherTest.WatcherPublicCertSecret))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(watcherTest.WatcherInternalCertSecret))
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

			// Simulate dbsync success
			th.SimulateJobSuccess(watcherTest.WatcherDBSync)

			// Simulate WatcherAPI deployment
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherAPIStatefulSet)

			// Simulate KeystoneEndpoint success
			keystone.SimulateKeystoneEndpointReady(watcherTest.WatcherKeystoneEndpointName)

			// Simulate WatcherApplier deployment
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherApplierStatefulSet)

			// Simulate WatcherDecisionEngine deployment
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherDecisionEngineStatefulSet)
		})
		It("should have the TLS Spec fields set", func() {
			Watcher := GetWatcher(watcherTest.Instance)
			Expect(*Watcher.Spec.APIServiceTemplate.TLS.API.Public.SecretName).Should(Equal("cert-watcher-public-svc"))
			Expect(*Watcher.Spec.APIServiceTemplate.TLS.API.Internal.SecretName).Should(Equal("cert-watcher-internal-svc"))
			Expect(Watcher.Spec.APIServiceTemplate.TLS.CaBundleSecretName).Should(Equal("combined-ca-bundle"))
		})
	})
	When("A Watcher instance with TLS at the route but not at pod level is created", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateWatcher(watcherTest.Instance, GetTLSIngressWatcherSpec()))
			DeferCleanup(k8sClient.Delete, ctx, CreateWatcherMessageBusSecret(watcherTest.Instance.Namespace, "rabbitmq-secret"))
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.Watcher.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					watcherTest.Instance.Namespace,
					*GetWatcher(watcherTest.Instance).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(watcherTest.WatcherAPI.Namespace))
			DeferCleanup(
				k8sClient.Delete, ctx, th.CreateSecret(
					types.NamespacedName{Namespace: watcherTest.Instance.Namespace, Name: "metric-storage-prometheus-endpoint"},
					map[string][]byte{
						"host": []byte("prometheus.example.com"),
						"port": []byte("9090"),
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

			// Simulate dbsync success
			th.SimulateJobSuccess(watcherTest.WatcherDBSync)

			// Simulate WatcherAPI deployment
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherAPIStatefulSet)

			// Simulate KeystoneEndpoint success
			keystone.SimulateKeystoneEndpointReady(watcherTest.WatcherKeystoneEndpointName)

			// Simulate WatcherApplier deployment
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherApplierStatefulSet)

			// Simulate WatcherDecisionEngine deployment
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherDecisionEngineStatefulSet)
		})
		It("should have the TLS Spec fields set", func() {
			Watcher := GetWatcher(watcherTest.Instance)
			Expect(Watcher.Spec.APIServiceTemplate.TLS.API.Public.SecretName).Should(BeNil())
			Expect(Watcher.Spec.APIServiceTemplate.TLS.API.Internal.SecretName).Should(BeNil())
			Expect(Watcher.Spec.APIServiceTemplate.TLS.CaBundleSecretName).Should(Equal("combined-ca-bundle"))
		})
	})
	When("A Watcher instance with TLS at the pod but not at the route is created", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateWatcher(watcherTest.Instance, GetTLSPodLevelWatcherSpec()))
			DeferCleanup(k8sClient.Delete, ctx, CreateWatcherMessageBusSecret(watcherTest.Instance.Namespace, "rabbitmq-secret"))
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.Watcher.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					watcherTest.Instance.Namespace,
					*GetWatcher(watcherTest.Instance).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(watcherTest.WatcherAPI.Namespace))
			DeferCleanup(
				k8sClient.Delete, ctx, th.CreateSecret(
					types.NamespacedName{Namespace: watcherTest.Instance.Namespace, Name: "metric-storage-prometheus-endpoint"},
					map[string][]byte{
						"host": []byte("prometheus.example.com"),
						"port": []byte("9090"),
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
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(watcherTest.WatcherPublicCertSecret))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(watcherTest.WatcherInternalCertSecret))
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

			// Simulate dbsync success
			th.SimulateJobSuccess(watcherTest.WatcherDBSync)
		})
	})

	When("Watcher is created with Topologies", func() {
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

			spec := GetNonDefaultWatcherSpec()
			spec["topologyRef"] = map[string]interface{}{"name": topologyRefAlt.Name}
			spec["apiServiceTemplate"] = map[string]interface{}{"topologyRef": map[string]interface{}{"name": topologyRefAPI.Name}}
			DeferCleanup(th.DeleteInstance, CreateWatcher(watcherTest.Instance, spec))
			DeferCleanup(k8sClient.Delete, ctx, CreateWatcherMessageBusSecret(watcherTest.Instance.Namespace, "rabbitmq-secret"))
			memcachedSpec := memcachedv1.MemcachedSpec{
				MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
					Replicas: ptr.To(int32(1)),
				},
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(watcherTest.Watcher.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(watcherTest.MemcachedNamespace)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					watcherTest.Instance.Namespace,
					*GetWatcher(watcherTest.Instance).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(watcherTest.WatcherAPI.Namespace))
			DeferCleanup(
				k8sClient.Delete, ctx, th.CreateSecret(
					types.NamespacedName{Namespace: watcherTest.Instance.Namespace, Name: "combined-ca-bundle"},
					map[string][]byte{
						"internal-ca-bundle.pem": []byte("some-b64-text"),
						"tls-ca-bundle.pem":      []byte("other-b64-text"),
					},
				))
			DeferCleanup(
				k8sClient.Delete, ctx, th.CreateSecret(
					types.NamespacedName{Namespace: watcherTest.Instance.Namespace, Name: "custom-prometheus-config"},
					map[string][]byte{
						"host":      []byte("customprometheus.example.com"),
						"port":      []byte("9092"),
						"ca_secret": []byte("combined-ca-bundle"),
						"ca_key":    []byte("internal-ca-bundle.pem"),
					},
				))

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

			// Simulate dbsync success
			th.SimulateJobSuccess(watcherTest.WatcherDBSync)

			// Simulate WatcherAPI deployment
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherAPIStatefulSet)

			// Simulate KeystoneEndpoint success
			keystone.SimulateKeystoneEndpointReady(watcherTest.WatcherKeystoneEndpointName)

			// Simulate WatcherApplier deployment
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherApplierStatefulSet)

			// Simulate WatcherDecisionEngine deployment
			th.SimulateStatefulSetReplicaReady(watcherTest.WatcherDecisionEngineStatefulSet)

		})

		It("should have the TopologyRefs fields with the expected values in the Spec", func() {
			Watcher := GetWatcher(watcherTest.Instance)
			Expect(Watcher.Spec.TopologyRef.Name).Should(Equal("watcher"))
			Expect(Watcher.Spec.APIServiceTemplate.TopologyRef.Name).Should(Equal("watcherapi"))
		})

		It("Should propagate topology to all Watcher SubCRs", func() {

			tpAlt := infra.GetTopology(types.NamespacedName{
				Name:      topologyRefAlt.Name,
				Namespace: topologyRefAlt.Namespace,
			})
			tpAPI := infra.GetTopology(types.NamespacedName{
				Name:      topologyRefAPI.Name,
				Namespace: topologyRefAPI.Namespace,
			})

			Expect(tpAlt.GetFinalizers()).To(HaveLen(2))
			finalizersAlt := tpAlt.GetFinalizers()

			Expect(tpAPI.GetFinalizers()).To(HaveLen(1))
			finalizersAPI := tpAPI.GetFinalizers()

			WatcherAPI := GetWatcherAPI(watcherTest.WatcherAPI)
			Expect(WatcherAPI.Spec.TopologyRef.Name).To(Equal("watcherapi"))
			Expect(WatcherAPI.Status.LastAppliedTopology).ToNot(BeNil())
			Expect(WatcherAPI.Status.LastAppliedTopology).To(Equal(&topologyRefAPI))
			Expect(finalizersAPI).To(ContainElement(
				fmt.Sprintf("openstack.org/watcherapi-%s", watcherTest.WatcherAPI.Name)))

			WatcherApplier := GetWatcherApplier(watcherTest.WatcherApplier)
			Expect(WatcherApplier.Spec.TopologyRef.Name).To(Equal("watcher"))
			Expect(WatcherApplier.Status.LastAppliedTopology).ToNot(BeNil())
			Expect(WatcherApplier.Status.LastAppliedTopology).To(Equal(&topologyRefAlt))
			Expect(finalizersAlt).To(ContainElement(
				fmt.Sprintf("openstack.org/watcherapplier-%s", watcherTest.WatcherApplier.Name)))

			WatcherDecisionEngine := GetWatcherDecisionEngine(watcherTest.WatcherDecisionEngine)
			Expect(WatcherDecisionEngine.Spec.TopologyRef.Name).To(Equal("watcher"))
			Expect(WatcherDecisionEngine.Status.LastAppliedTopology).ToNot(BeNil())
			Expect(WatcherDecisionEngine.Status.LastAppliedTopology).To(Equal(&topologyRefAlt))
			Expect(finalizersAlt).To(ContainElement(
				fmt.Sprintf("openstack.org/watcherdecisionengine-%s", watcherTest.WatcherDecisionEngine.Name)))

			// Global status Ready
			th.ExpectCondition(
				watcherTest.Instance,
				ConditionGetterFunc(WatcherConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			th.ExpectCondition(
				watcherTest.WatcherAPI,
				ConditionGetterFunc(WatcherAPIConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionTrue,
			)

			th.ExpectCondition(
				watcherTest.WatcherApplier,
				ConditionGetterFunc(WatcherApplierConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionTrue,
			)

			th.ExpectCondition(
				watcherTest.WatcherDecisionEngine,
				ConditionGetterFunc(WatcherDecisionEngineConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionTrue,
			)

			// Assert that the statefulsets have the topology
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

		})
	})

})
