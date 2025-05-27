/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/statefulset"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"

	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
	watcherv1beta1 "github.com/openstack-k8s-operators/watcher-operator/api/v1beta1"

	"github.com/openstack-k8s-operators/watcher-operator/pkg/watcher"
	"github.com/openstack-k8s-operators/watcher-operator/pkg/watcherapi"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
)

// WatcherAPIReconciler reconciles a WatcherAPI object
type WatcherAPIReconciler struct {
	ReconcilerBase
}

// GetLogger returns a logger object with a prefix of "controller.name" and
// additional controller context fields
func (r *WatcherAPIReconciler) GetLogger(ctx context.Context) logr.Logger {
	return log.FromContext(ctx).WithName("Controllers").WithName("WatcherAPI")
}

//+kubebuilder:rbac:groups=watcher.openstack.org,resources=watcherapis,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=watcher.openstack.org,resources=watcherapis/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=watcher.openstack.org,resources=watcherapis/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete;
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete;
//+kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis,verbs=get;list;watch;
//+kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneservices,verbs=get;list;watch;create;update;patch;delete;
//+kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneendpoints,verbs=get;list;watch;create;update;patch;delete;
//+kubebuilder:rbac:groups=memcached.openstack.org,resources=memcacheds,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=memcached.openstack.org,resources=memcacheds/finalizers,verbs=update;patch
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete;
//+kubebuilder:rbac:groups=topology.openstack.org,resources=topologies,verbs=get;list;watch;update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.3/pkg/reconcile
func (r *WatcherAPIReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {

	Log := r.GetLogger(ctx)
	instance := &watcherv1beta1.WatcherAPI{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected.
			// For additional cleanup logic use finalizers. Return and don't requeue.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	Log.Info(fmt.Sprintf("Reconciling WatcherAPI instance '%s'", instance.Name))

	helper, err := helper.NewHelper(
		instance,
		r.Client,
		r.Kclient,
		r.Scheme,
		Log,
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	isNewInstance := instance.Status.Conditions == nil
	// Save a copy of the conditions so that we can restore the LastTransitionTime
	// when a condition's state doesn't change.
	savedConditions := instance.Status.Conditions.DeepCopy()

	// Always patch the instance status when exiting this function so we can
	// persist any changes.
	defer func() {
		// Don't update the status, if reconciler Panics
		if r := recover(); r != nil {
			Log.Info(fmt.Sprintf("panic during reconcile %v\n", r))
			panic(r)
		}
		condition.RestoreLastTransitionTimes(
			&instance.Status.Conditions, savedConditions)
		if instance.Status.Conditions.IsUnknown(condition.ReadyCondition) {
			instance.Status.Conditions.Set(
				instance.Status.Conditions.Mirror(condition.ReadyCondition))
		}
		err := helper.PatchInstance(ctx, instance)
		if err != nil {
			_err = err
			return
		}
	}()

	err = r.initStatus(instance)
	if err != nil {
		return ctrl.Result{}, nil
	}

	// If we're not deleting this and the service object doesn't have our finalizer, add it.
	if instance.DeletionTimestamp.IsZero() && controllerutil.AddFinalizer(instance, helper.GetFinalizer()) || isNewInstance {
		return ctrl.Result{}, nil
	}

	// Handle service delete
	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance, helper)
	}

	configVars := make(map[string]env.Setter)
	// check for required OpenStack secret holding passwords for service/admin user and add hash to the vars map
	Log.Info(fmt.Sprintf("[API] Get secret 1 '%s'", instance.Spec.Secret))
	secretHash, result, secret, err := ensureSecret(
		ctx,
		types.NamespacedName{Namespace: instance.Namespace, Name: instance.Spec.Secret},
		[]string{
			instance.Spec.PasswordSelectors.Service,
			TransportURLSelector,
			DatabaseAccount,
			DatabaseUsername,
			DatabaseHostname,
			DatabasePassword,
			watcher.GlobalCustomConfigFileName,
		},
		helper.GetClient(),
		&instance.Status.Conditions,
		r.RequeueTimeout,
	)
	if (err != nil || result != ctrl.Result{}) {
		return result, err
	}

	configVars[instance.Spec.Secret] = env.SetValue(secretHash)

	// Prometheus config secret

	hashPrometheus, _, prometheusSecret, err := ensureSecret(
		ctx,
		types.NamespacedName{Namespace: instance.Namespace, Name: instance.Spec.PrometheusSecret},
		[]string{
			PrometheusHost,
			PrometheusPort,
		},
		helper.GetClient(),
		&instance.Status.Conditions,
		r.RequeueTimeout,
	)
	if err != nil || hashPrometheus == "" {
		// Empty hash means that there is some problem retrieving the key from the secret
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.RequestedReason,
			condition.SeverityWarning,
			watcherv1beta1.WatcherPrometheusSecretErrorMessage))
		return ctrl.Result{}, errors.New("error retrieving required data from prometheus secret")
	}

	configVars[instance.Spec.PrometheusSecret] = env.SetValue(hashPrometheus)

	// all our input checks out so report InputReady
	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)

	memcached, err := ensureMemcached(ctx, helper, instance.Namespace, instance.Spec.MemcachedInstance, &instance.Status.Conditions)

	if err != nil {
		return ctrl.Result{}, err
	}
	// Add finalizer to Memcached to prevent it from being deleted now that we're using it
	if controllerutil.AddFinalizer(memcached, helper.GetFinalizer()) {
		err := helper.GetClient().Update(ctx, memcached)
		if err != nil {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.MemcachedReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.MemcachedReadyErrorMessage,
				err.Error()))
			return ctrl.Result{}, err
		}
	}

	err = r.generateServiceConfigs(ctx, instance, secret, prometheusSecret, memcached, helper, &configVars)
	if err != nil {
		return ctrl.Result{}, err
	}

	Log.Info(fmt.Sprintf("[API] Getting input hash '%s'", instance.Name))

	//
	// Handle Topology
	//

	topology, err := ensureTopology(
		ctx,
		helper,
		instance,      // topologyHandler
		instance.Name, // finalizer
		&instance.Status.Conditions,
		labels.GetLabelSelector(getAPIServiceLabels()),
	)

	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.TopologyReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.TopologyReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, fmt.Errorf("waiting for Topology requirements: %w", err)
	}

	//
	// TLS input validation
	//
	// Validate the CA cert secret if provided
	if instance.Spec.TLS.CaBundleSecretName != "" {
		hash, err := tls.ValidateCACertSecret(
			ctx,
			helper.GetClient(),
			types.NamespacedName{
				Name:      instance.Spec.TLS.CaBundleSecretName,
				Namespace: instance.Namespace,
			},
		)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				instance.Status.Conditions.Set(condition.FalseCondition(
					condition.TLSInputReadyCondition,
					condition.RequestedReason,
					condition.SeverityInfo,
					fmt.Sprintf(condition.TLSInputReadyWaitingMessage, instance.Spec.TLS.CaBundleSecretName),
				))
				return ctrl.Result{}, nil
			}
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.TLSInputReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.TLSInputErrorMessage,
				err.Error()))
			return ctrl.Result{}, err
		}

		if hash != "" {
			configVars[tls.CABundleKey] = env.SetValue(hash)
		}
	}
	// Validate API certs secrets
	certsHash, err := instance.Spec.TLS.API.ValidateCertSecrets(ctx, helper, instance.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.TLSInputReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				fmt.Sprintf(condition.TLSInputReadyWaitingMessage, err.Error()),
			))
			return ctrl.Result{}, nil
		}
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.TLSInputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.TLSInputErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	configVars[tls.TLSHashName] = env.SetValue(certsHash)

	// all cert input checks out so report TLSInputReady
	instance.Status.Conditions.MarkTrue(condition.TLSInputReadyCondition, condition.InputReadyMessage)

	//
	// create hash over all the different input resources to identify if any those changed
	// and a restart/recreate is required.
	//

	inputHash, hashChanged, errorHash := r.createHashOfInputHashes(ctx, instance, configVars)
	if errorHash != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			errorHash.Error()))
		return ctrl.Result{}, errorHash
	} else if hashChanged {
		// Hash changed and instance status should be updated (which will be done by main defer func),
		// so we need to return and reconcile again
		return ctrl.Result{}, nil
	}

	instance.Status.Conditions.MarkTrue(condition.ServiceConfigReadyCondition, condition.ServiceConfigReadyMessage)

	result, err = r.ensureDeployment(ctx, helper, instance, prometheusSecret, inputHash, topology)
	if (err != nil || result != ctrl.Result{}) {
		return result, err
	}

	// Only expose the service if the deployment succeeded
	if !instance.Status.Conditions.IsTrue(condition.DeploymentReadyCondition) {
		Log.Info("Waiting for the Deployment to become Ready before exposing the service in Keystone")
		return ctrl.Result{}, nil
	}

	apiEndpoints, result, err := r.ensureServiceExposed(ctx, helper, instance)
	if (err != nil || result != ctrl.Result{}) {
		// We can ignore RequeueAfter as we are watching the Service resource
		// but we have to return while waiting for the service to be exposed
		return ctrl.Result{}, err
	}

	result, err = r.ensureKeystoneEndpoint(ctx, helper, instance, apiEndpoints)
	if (err != nil || result != ctrl.Result{}) {
		// We can ignore RequeueAfter as we are watching the KeystoneEndpoint
		// resource
		return result, err
	}

	// We reached the end of the Reconcile, update the Ready condition based on
	// the sub conditions
	if instance.Status.Conditions.AllSubConditionIsTrue() {
		instance.Status.Conditions.MarkTrue(
			condition.ReadyCondition, condition.ReadyMessage)
	}

	Log.Info(fmt.Sprintf("Successfully reconciled WatcherAPI instance '%s'", instance.Name))
	return ctrl.Result{}, nil
}

// generateServiceConfigs - create Secret which holds the service configuration
func (r *WatcherAPIReconciler) generateServiceConfigs(
	ctx context.Context, instance *watcherv1beta1.WatcherAPI,
	secret corev1.Secret,
	prometheusSecret corev1.Secret,
	memcachedInstance *memcachedv1.Memcached,
	helper *helper.Helper, envVars *map[string]env.Setter,
) error {
	Log := r.GetLogger(ctx)
	Log.Info("generateServiceConfigs - reconciling")

	labels := labels.GetLabels(instance, labels.GetGroupLabel(WatcherAPILabelPrefix), map[string]string{})

	keystoneAPI, err := keystonev1.GetKeystoneAPI(ctx, helper, instance.Namespace, map[string]string{})
	// KeystoneAPI not available we should not aggregate the error and continue
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			"keystoneAPI not found"))
		return err
	}
	keystoneInternalURL, err := keystoneAPI.GetEndpoint(endpoint.EndpointInternal)
	if err != nil {
		return err
	}

	databaseAccount := string(secret.Data[DatabaseAccount])
	db, err := mariadbv1.GetDatabaseByNameAndAccount(ctx, helper, watcher.DatabaseCRName, databaseAccount, instance.Namespace)
	if err != nil {
		return err
	}
	// customData hold any customization for the service.
	var tlsCfg *tls.Service
	if instance.Spec.TLS.Ca.CaBundleSecretName != "" {
		tlsCfg = &tls.Service{}
	}
	// customData hold any customization for the service.
	customData := map[string]string{
		watcher.GlobalCustomConfigFileName:  string(secret.Data[watcher.GlobalCustomConfigFileName]),
		watcher.ServiceCustomConfigFileName: instance.Spec.CustomServiceConfig,
		"my.cnf":                            db.GetDatabaseClientConfig(tlsCfg), //(mschuppert) for now just get the default my.cnf
	}

	databaseUsername := string(secret.Data[DatabaseUsername])
	databaseHostname := string(secret.Data[DatabaseHostname])
	databasePassword := string(secret.Data[DatabasePassword])

	prometheusHost := string(prometheusSecret.Data[PrometheusHost])
	prometheusPort := string(prometheusSecret.Data[PrometheusPort])
	prometheusCaCertSecret := string(prometheusSecret.Data[PrometheusCaCertSecret])
	prometheusCaCertKey := string(prometheusSecret.Data[PrometheusCaCertKey])

	var prometheusCaCertPath string
	if prometheusCaCertSecret != "" && prometheusCaCertKey != "" {
		prometheusCaCertPath = filepath.Join(watcher.PrometheusCaCertFolderPath, prometheusCaCertKey)
	}

	var CaFilePath string
	if instance.Spec.TLS.CaBundleSecretName != "" {
		CaFilePath = tls.DownstreamTLSCABundlePath
	}
	templateParameters := map[string]interface{}{
		"DatabaseConnection": fmt.Sprintf("mysql+pymysql://%s:%s@%s/%s?read_default_file=/etc/my.cnf",
			databaseUsername,
			databasePassword,
			databaseHostname,
			watcher.DatabaseName,
		),
		"KeystoneAuthURL":          keystoneInternalURL,
		"ServicePassword":          string(secret.Data[instance.Spec.PasswordSelectors.Service]),
		"ServiceUser":              instance.Spec.ServiceUser,
		"TransportURL":             string(secret.Data[TransportURLSelector]),
		"MemcachedServers":         memcachedInstance.GetMemcachedServerListString(),
		"MemcachedServersWithInet": memcachedInstance.GetMemcachedServerListWithInetString(),
		"MemcachedTLS":             memcachedInstance.GetMemcachedTLSSupport(),
		"LogFile":                  fmt.Sprintf("%s%s.log", watcher.WatcherLogPath, instance.Name),
		"APIPublicPort":            fmt.Sprintf("%d", watcher.WatcherPublicPort),
		"CaFilePath":               CaFilePath,
		"PrometheusHost":           prometheusHost,
		"PrometheusPort":           prometheusPort,
		"PrometheusCaCertPath":     prometheusCaCertPath,
	}

	// create httpd  vhost template parameters
	httpdVhostConfig := map[string]interface{}{}
	for _, endpt := range []service.Endpoint{service.EndpointInternal, service.EndpointPublic} {
		endptConfig := map[string]interface{}{}
		endptConfig["ServerName"] = fmt.Sprintf("%s-%s.%s.svc", watcher.ServiceName, endpt.String(), instance.Namespace)
		endptConfig["TLS"] = false // default TLS to false, and set it below to true if enabled
		if instance.Spec.TLS.API.Enabled(endpt) {
			endptConfig["TLS"] = true
			endptConfig["SSLCertificateFile"] = fmt.Sprintf("/etc/pki/tls/certs/%s.crt", endpt.String())
			endptConfig["SSLCertificateKeyFile"] = fmt.Sprintf("/etc/pki/tls/private/%s.key", endpt.String())
		}
		endptConfig["Port"] = fmt.Sprintf("%d", watcher.WatcherPublicPort)
		httpdVhostConfig[endpt.String()] = endptConfig
	}
	templateParameters["VHosts"] = httpdVhostConfig

	return GenerateConfigsGeneric(ctx, helper, instance, envVars, templateParameters, customData, labels, false)
}

func (r *WatcherAPIReconciler) ensureDeployment(
	ctx context.Context,
	helper *helper.Helper,
	instance *watcherv1beta1.WatcherAPI,
	prometheusSecret corev1.Secret,
	configHash string,
	topology *topologyv1.Topology,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info(fmt.Sprintf("Defining WatcherAPI deployment '%s'", instance.Name))

	// If there prometheus config is providing CA cert a volume must be mounted
	prometheusCaCertSecret := string(prometheusSecret.Data[PrometheusCaCertSecret])
	prometheusCaCertKey := string(prometheusSecret.Data[PrometheusCaCertKey])
	prometheusCaCert := make(map[string]string)
	if prometheusCaCertSecret != "" && prometheusCaCertKey != "" {
		prometheusCaCert = map[string]string{
			"casecret_key":  prometheusCaCertKey,
			"casecret_name": prometheusCaCertSecret,
		}

	}

	// define a new StatefulSet object
	statefulSetDef, err := watcherapi.StatefulSet(instance, configHash, prometheusCaCert, getAPIServiceLabels(), topology)
	if err != nil {
		Log.Error(err, "Defining statefulSet failed")
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DeploymentReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	Log.Info(fmt.Sprintf("Getting statefulSet '%s'", instance.Name))
	statefulSetObject := statefulset.NewStatefulSet(statefulSetDef, r.RequeueTimeout)
	ctrlResult, errorResult := statefulSetObject.CreateOrPatch(ctx, helper)
	if errorResult != nil {
		Log.Error(err, "Deployment failed")
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DeploymentReadyErrorMessage,
			errorResult.Error()))
		return ctrlResult, errorResult
	} else if (ctrlResult != ctrl.Result{}) {
		Log.Info("Deployment in progress")
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DeploymentReadyRunningMessage))
		return ctrlResult, nil
	}

	Log.Info(fmt.Sprintf("Got statefulSet '%s'", instance.Name))
	statefulSet := statefulSetObject.GetStatefulSet()
	if statefulSet.Generation == statefulSet.Status.ObservedGeneration {
		instance.Status.ReadyCount = statefulSet.Status.ReadyReplicas
	}

	if instance.Status.ReadyCount == *instance.Spec.Replicas && statefulSet.Generation == statefulSet.Status.ObservedGeneration {
		Log.Info("Deployment is ready")
		instance.Status.Conditions.MarkTrue(condition.DeploymentReadyCondition, condition.DeploymentReadyMessage)
	} else {
		Log.Info("Deployment not ready")
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DeploymentReadyRunningMessage,
		))
	}

	return ctrl.Result{}, nil
}

func (r *WatcherAPIReconciler) ensureServiceExposed(
	ctx context.Context,
	helper *helper.Helper,
	instance *watcherv1beta1.WatcherAPI,
) (map[string]string, ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info(fmt.Sprintf("Defining WatcherAPI services '%s'", instance.Name))

	ports := map[service.Endpoint]endpoint.Data{
		service.EndpointPublic: {
			Port: watcher.WatcherPublicPort,
		},
		service.EndpointInternal: {
			Port: watcher.WatcherPublicPort,
		},
	}
	apiEndpoints := make(map[string]string)

	for endpointType, data := range ports {
		endpointTypeStr := string(endpointType)
		endpointName := watcher.ServiceName + "-" + endpointTypeStr
		svcOverride := instance.Spec.Override.Service[endpointType]
		if svcOverride.EmbeddedLabelsAnnotations == nil {
			svcOverride.EmbeddedLabelsAnnotations = &service.EmbeddedLabelsAnnotations{}
		}

		exportLabels := util.MergeStringMaps(
			getAPIServiceLabels(),
			map[string]string{
				service.AnnotationEndpointKey: endpointTypeStr,
			},
		)

		// create the service
		svc, err := service.NewService(
			service.GenericService(&service.GenericServiceDetails{
				Name:      endpointName,
				Namespace: instance.Namespace,
				Labels:    exportLabels,
				Selector:  getAPIServiceLabels(),
				Port: service.GenericServicePort{
					Name:     endpointName,
					Port:     data.Port,
					Protocol: corev1.ProtocolTCP,
				},
			}),
			5,
			&svcOverride.OverrideSpec,
		)
		if err != nil {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.CreateServiceReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.CreateServiceReadyErrorMessage,
				err.Error(),
			))
			return nil, ctrl.Result{}, err
		}

		svc.AddAnnotation(map[string]string{
			service.AnnotationEndpointKey: endpointTypeStr,
		})

		// add Annotation to whether creating an ingress is required or not
		if endpointType == service.EndpointPublic && svc.GetServiceType() == corev1.ServiceTypeClusterIP {
			svc.AddAnnotation(map[string]string{
				service.AnnotationIngressCreateKey: "true",
			})
		} else {
			svc.AddAnnotation(map[string]string{
				service.AnnotationIngressCreateKey: "false",
			})
			if svc.GetServiceType() == corev1.ServiceTypeLoadBalancer {
				svc.AddAnnotation(map[string]string{
					// add annotation to register service name in dnsmasq
					service.AnnotationHostnameKey: svc.GetServiceHostname(),
				})
			}
		}

		ctrlResult, err := svc.CreateOrPatch(ctx, helper)
		if err != nil {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.CreateServiceReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.CreateServiceReadyErrorMessage,
				err.Error(),
			))

			return nil, ctrlResult, err
		} else if (ctrlResult != ctrl.Result{}) {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.CreateServiceReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.CreateServiceReadyRunningMessage,
			))

			return nil, ctrlResult, nil
		}

		// if TLS is enabled
		if instance.Spec.TLS.API.Enabled(endpointType) {
			// set endpoint protocol to https
			data.Protocol = ptr.To(service.ProtocolHTTPS)
		}

		apiEndpoints[endpointTypeStr], err = svc.GetAPIEndpoint(
			svcOverride.EndpointURL,
			data.Protocol,
			"",
		)
		if err != nil {
			return nil, ctrl.Result{}, err
		}
	}

	instance.Status.Conditions.MarkTrue(condition.CreateServiceReadyCondition, condition.CreateServiceReadyMessage)

	return apiEndpoints, ctrl.Result{}, nil
}

func (r *WatcherAPIReconciler) ensureKeystoneEndpoint(
	ctx context.Context,
	helper *helper.Helper,
	instance *watcherv1beta1.WatcherAPI,
	apiEndpoints map[string]string,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info(fmt.Sprintf("Defining WatcherAPI KeystoneEndpoint '%s'", instance.Name))

	endpointSpec := keystonev1.KeystoneEndpointSpec{
		ServiceName: watcher.ServiceName,
		Endpoints:   apiEndpoints,
	}
	endpoint := keystonev1.NewKeystoneEndpoint(
		watcher.ServiceName,
		instance.Namespace,
		endpointSpec,
		getAPIServiceLabels(),
		r.RequeueTimeout,
	)
	ctrlResult, err := endpoint.CreateOrPatch(ctx, helper)
	if err != nil {
		return ctrlResult, err
	}

	c := endpoint.GetConditions().Mirror(condition.KeystoneEndpointReadyCondition)
	if c != nil {
		instance.Status.Conditions.Set(c)
	}

	return ctrlResult, nil
}

func (r *WatcherAPIReconciler) ensureKeystoneEndpointDeletion(
	ctx context.Context,
	helper *helper.Helper,
	instance *watcherv1beta1.WatcherAPI,
) error {
	// Remove the finalizer from our KeystoneEndpoint CR
	// This is oddly added automatically when we created KeystoneEndpoint but
	// we need to remove it manually
	Log := r.GetLogger(ctx)

	endpoint, err := keystonev1.GetKeystoneEndpointWithName(ctx, helper, watcher.ServiceName, instance.Namespace)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return err
	}

	if k8s_errors.IsNotFound(err) {
		// Nothing to do as it was never created
		return nil
	}

	updated := controllerutil.RemoveFinalizer(endpoint, helper.GetFinalizer())
	if !updated {
		// No finalizer to remove
		return nil
	}

	if err = helper.GetClient().Update(ctx, endpoint); err != nil && !k8s_errors.IsNotFound(err) {
		return err
	}
	Log.Info("Removed finalizer from WatcherAPI KeystoneEndpoint")

	return nil
}

func (r *WatcherAPIReconciler) reconcileDelete(ctx context.Context, instance *watcherv1beta1.WatcherAPI, helper *helper.Helper) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info(fmt.Sprintf("Reconcile Service '%s' delete started", instance.Name))

	err := r.ensureKeystoneEndpointDeletion(ctx, helper, instance)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Remove our finalizer from Memcached
	memcached, err := memcachedv1.GetMemcachedByName(ctx, helper, instance.Spec.MemcachedInstance, instance.Namespace)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}
	if memcached != nil {
		if controllerutil.RemoveFinalizer(memcached, helper.GetFinalizer()) {
			err := helper.GetClient().Update(ctx, memcached)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	// Remove finalizer on the Topology CR
	if ctrlResult, err := topologyv1.EnsureDeletedTopologyRef(
		ctx,
		helper,
		instance.Status.LastAppliedTopology,
		instance.Name,
	); err != nil {
		return ctrlResult, err
	}

	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
	Log.Info(fmt.Sprintf("Reconciled Service '%s' delete successfully", instance.Name))
	return ctrl.Result{}, nil
}

func (r *WatcherAPIReconciler) initStatus(instance *watcherv1beta1.WatcherAPI) error {

	cl := condition.CreateList(
		// Mark ReadyCondition as Unknown from the beginning, because the
		// Reconcile function is in progress. If this condition is not marked
		// as True and is still in the "Unknown" state, we `Mirror(` the actual
		// failure/in-progress operation
		condition.UnknownCondition(condition.ReadyCondition, condition.InitReason, condition.ReadyInitMessage),
		condition.UnknownCondition(condition.InputReadyCondition, condition.InitReason, condition.InputReadyInitMessage),
		condition.UnknownCondition(condition.TLSInputReadyCondition, condition.InitReason, condition.InputReadyInitMessage),
		condition.UnknownCondition(condition.ServiceConfigReadyCondition, condition.InitReason, condition.ServiceConfigReadyInitMessage),
		condition.UnknownCondition(condition.MemcachedReadyCondition, condition.InitReason, condition.MemcachedReadyInitMessage),
		condition.UnknownCondition(condition.DeploymentReadyCondition, condition.InitReason, condition.DeploymentReadyInitMessage),
		condition.UnknownCondition(condition.CreateServiceReadyCondition, condition.InitReason, condition.CreateServiceReadyInitMessage),
		condition.UnknownCondition(condition.KeystoneEndpointReadyCondition, condition.InitReason, "KeystoneEndpoint not created"),
	)

	// Init Topology condition if there's a reference
	if instance.Spec.TopologyRef != nil {
		c := condition.UnknownCondition(condition.TopologyReadyCondition, condition.InitReason, condition.TopologyReadyInitMessage)
		cl.Set(c)
	}

	instance.Status.Conditions.Init(&cl)

	// Update the lastObserved generation before evaluating conditions
	instance.Status.ObservedGeneration = instance.Generation

	if instance.Status.Hash == nil {
		instance.Status.Hash = map[string]string{}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WatcherAPIReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// index passwordSecretField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &watcherv1beta1.WatcherAPI{}, passwordSecretField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*watcherv1beta1.WatcherAPI)
		if cr.Spec.Secret == "" {
			return nil
		}
		return []string{cr.Spec.Secret}
	}); err != nil {
		return err
	}

	// index prometheusSecretField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &watcherv1beta1.WatcherAPI{}, prometheusSecretField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*watcherv1beta1.WatcherAPI)
		if cr.Spec.PrometheusSecret == "" {
			return nil
		}
		return []string{cr.Spec.PrometheusSecret}
	}); err != nil {
		return err
	}

	// index topologyField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &watcherv1beta1.WatcherAPI{}, topologyField, func(rawObj client.Object) []string {
		// Extract the topology name from the spec, if one is provided
		cr := rawObj.(*watcherv1beta1.WatcherAPI)
		if cr.Spec.TopologyRef == nil {
			return nil
		}
		return []string{cr.Spec.TopologyRef.Name}
	}); err != nil {
		return err
	}

	// index tlsAPIInternalField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &watcherv1beta1.WatcherAPI{}, tlsAPIInternalField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*watcherv1beta1.WatcherAPI)
		if cr.Spec.TLS.API.Internal.SecretName == nil {
			return nil
		}
		return []string{*cr.Spec.TLS.API.Internal.SecretName}
	}); err != nil {
		return err
	}
	// index tlsAPIPublicField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &watcherv1beta1.WatcherAPI{}, tlsAPIPublicField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*watcherv1beta1.WatcherAPI)
		if cr.Spec.TLS.API.Public.SecretName == nil {
			return nil
		}
		return []string{*cr.Spec.TLS.API.Public.SecretName}
	}); err != nil {
		return err
	}
	// index caBundleSecretNameField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &watcherv1beta1.WatcherAPI{}, caBundleSecretNameField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*watcherv1beta1.WatcherAPI)

		if cr.Spec.TLS.CaBundleSecretName == "" {
			return nil
		}
		return []string{cr.Spec.TLS.CaBundleSecretName}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&watcherv1beta1.WatcherAPI{}).
		Owns(&corev1.Secret{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&keystonev1.KeystoneEndpoint{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForSrc),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(&topologyv1.Topology{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForSrc),
			builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}

func (r *WatcherAPIReconciler) findObjectsForSrc(ctx context.Context, src client.Object) []reconcile.Request {
	requests := []reconcile.Request{}

	l := log.FromContext(ctx).WithName("Controllers").WithName("WatcherAPI")

	for _, field := range apiWatchFields {
		crList := &watcherv1beta1.WatcherAPIList{}
		listOps := &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(field, src.GetName()),
			Namespace:     src.GetNamespace(),
		}
		err := r.Client.List(ctx, crList, listOps)
		if err != nil {
			l.Error(err, fmt.Sprintf("listing %s for field: %s - %s", crList.GroupVersionKind().Kind, field, src.GetNamespace()))
			return requests
		}

		for _, item := range crList.Items {
			l.Info(fmt.Sprintf("input source %s changed, reconcile: %s - %s", src.GetName(), item.GetName(), item.GetNamespace()))

			requests = append(requests,
				reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      item.GetName(),
						Namespace: item.GetNamespace(),
					},
				},
			)
		}
	}

	return requests
}

func (r *WatcherAPIReconciler) createHashOfInputHashes(
	ctx context.Context,
	instance *watcherv1beta1.WatcherAPI,
	envVars map[string]env.Setter,
) (string, bool, error) {
	Log := r.GetLogger(ctx)
	var hashMap map[string]string
	changed := false
	mergedMapVars := env.MergeEnvs([]corev1.EnvVar{}, envVars)
	hash, err := util.ObjectHash(mergedMapVars)
	if err != nil {
		return hash, changed, err
	}
	if hashMap, changed = util.SetHash(instance.Status.Hash, common.InputHashName, hash); changed {
		instance.Status.Hash = hashMap
		Log.Info(fmt.Sprintf("Input maps hash %s - %s", common.InputHashName, hash))
	}
	return hash, changed, nil
}
