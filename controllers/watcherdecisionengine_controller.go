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
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	"github.com/openstack-k8s-operators/lib-common/modules/common/statefulset"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
	watcherv1beta1 "github.com/openstack-k8s-operators/watcher-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/watcher-operator/pkg/watcher"
	"github.com/openstack-k8s-operators/watcher-operator/pkg/watcherdecisionengine"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
)

// WatcherDecisionEngineReconciler reconciles a WatcherDecisionEngine object
type WatcherDecisionEngineReconciler struct {
	ReconcilerBase
}

// GetLogger returns a logger object with a prefix of "controller.name" and
// additional controller context fields
func (r *WatcherDecisionEngineReconciler) GetLogger(ctx context.Context) logr.Logger {
	return log.FromContext(ctx).WithName("Controllers").WithName("WatcherDecisionEngine")
}

//+kubebuilder:rbac:groups=watcher.openstack.org,resources=watcherdecisionengines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=watcher.openstack.org,resources=watcherdecisionengines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=watcher.openstack.org,resources=watcherdecisionengines/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete;

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the WatcherDecisionEngine object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *WatcherDecisionEngineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	Log := r.GetLogger(ctx)

	instance := &watcherv1beta1.WatcherDecisionEngine{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Object not found
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}
	Log.Info(fmt.Sprintf("Reconciling WatcherDecisionEngine %s", instance.Name))

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

	// initialize status if Conditions is nil, but do not reset if it already
	// exists
	isNewInstance := instance.Status.Conditions == nil
	if isNewInstance {
		instance.Status.Conditions = condition.Conditions{}
	}

	// Save a copy of the condtions so that we can restore the LastTransitionTime
	// when a condition's state doesn't change.
	savedConditions := instance.Status.Conditions.DeepCopy()

	// Always patch the instance status when exiting this function so we can
	// persist any changes.
	defer func() {
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
	Log.Info(fmt.Sprintf("[DecisionEngine] Get secret 1 '%s'", instance.Spec.Secret))
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

	Log.Info(fmt.Sprintf("[DecisionEngine] Getting input hash '%s'", instance.Name))
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

	result, err = r.ensureDeployment(ctx, helper, instance, prometheusSecret, inputHash)
	if err != nil {
		return result, err
	}

	// We reached the end of the Reconcile, update the Ready condition based on
	// the sub conditions
	if instance.Status.Conditions.AllSubConditionIsTrue() {
		instance.Status.Conditions.MarkTrue(
			condition.ReadyCondition, condition.ReadyMessage)
	}

	Log.Info(fmt.Sprintf("Successfully reconciled WatcherDecisionEngine instance '%s'", instance.Name))
	return ctrl.Result{}, nil
}

func (r *WatcherDecisionEngineReconciler) initStatus(instance *watcherv1beta1.WatcherDecisionEngine) error {

	cl := condition.CreateList(
		// Mark ReadyCondition as Unknown from the beginning, because the
		// Reconcile function is in progress. If this condition is not marked
		// as True and is still in the "Unknown" state, we `Mirror(` the actual
		// failure/in-progress operation
		condition.UnknownCondition(condition.ReadyCondition, condition.InitReason, condition.ReadyInitMessage),
		condition.UnknownCondition(condition.InputReadyCondition, condition.InitReason, condition.InputReadyInitMessage),
		condition.UnknownCondition(condition.ServiceConfigReadyCondition, condition.InitReason, condition.ServiceConfigReadyInitMessage),
		condition.UnknownCondition(condition.MemcachedReadyCondition, condition.InitReason, condition.MemcachedReadyInitMessage),
		condition.UnknownCondition(condition.DeploymentReadyCondition, condition.InitReason, condition.DeploymentReadyInitMessage),
	)

	instance.Status.Conditions.Init(&cl)

	// Update the lastObserved generation before evaluating conditions
	instance.Status.ObservedGeneration = instance.Generation

	if instance.Status.Hash == nil {
		instance.Status.Hash = map[string]string{}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WatcherDecisionEngineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// index passwordSecretField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &watcherv1beta1.WatcherDecisionEngine{}, passwordSecretField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*watcherv1beta1.WatcherDecisionEngine)
		if cr.Spec.Secret == "" {
			return nil
		}
		return []string{cr.Spec.Secret}
	}); err != nil {
		return err
	}

	// index prometheusSecretField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &watcherv1beta1.WatcherDecisionEngine{}, prometheusSecretField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*watcherv1beta1.WatcherDecisionEngine)
		if cr.Spec.PrometheusSecret == "" {
			return nil
		}
		return []string{cr.Spec.PrometheusSecret}
	}); err != nil {
		return err
	}
	// index caBundleSecretNameField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &watcherv1beta1.WatcherDecisionEngine{}, caBundleSecretNameField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*watcherv1beta1.WatcherDecisionEngine)

		if cr.Spec.TLS.CaBundleSecretName == "" {
			return nil
		}
		return []string{cr.Spec.TLS.CaBundleSecretName}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&watcherv1beta1.WatcherDecisionEngine{}).
		Owns(&corev1.Secret{}).
		Owns(&appsv1.StatefulSet{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForSrc),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}

func (r *WatcherDecisionEngineReconciler) findObjectsForSrc(ctx context.Context, src client.Object) []reconcile.Request {
	requests := []reconcile.Request{}

	l := log.FromContext(ctx).WithName("Controllers").WithName("WatcherDecisionEngine")

	for _, field := range decisionEngineWatchFields {
		crList := &watcherv1beta1.WatcherDecisionEngineList{}
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

// generateServiceConfigs - create Secret which holds the service configuration
// NOTE - jgilaber this function is WIP, currently implements a fraction of its
// functionality and will be expanded of further iteration to actually generate
// the service configs
func (r *WatcherDecisionEngineReconciler) generateServiceConfigs(
	ctx context.Context, instance *watcherv1beta1.WatcherDecisionEngine,
	secret corev1.Secret,
	prometheusSecret corev1.Secret,
	memcachedInstance *memcachedv1.Memcached,
	helper *helper.Helper, envVars *map[string]env.Setter,
) error {
	Log := r.GetLogger(ctx)
	Log.Info("generateServiceConfigs - reconciling")

	labels := labels.GetLabels(instance, labels.GetGroupLabel(WatcherDecisionEngineLabelPrefix), map[string]string{})

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
	if instance.Spec.TLS.CaBundleSecretName != "" {
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
		"APIPublicPort":            fmt.Sprintf("%d", watcher.WatcherPublicPort),
		"CaFilePath":               CaFilePath,
		"PrometheusHost":           prometheusHost,
		"PrometheusPort":           prometheusPort,
		"PrometheusCaCertPath":     prometheusCaCertPath,
	}

	return GenerateConfigsGeneric(ctx, helper, instance, envVars, templateParameters, customData, labels, false)
}

func (r *WatcherDecisionEngineReconciler) ensureDeployment(
	ctx context.Context,
	h *helper.Helper,
	instance *watcherv1beta1.WatcherDecisionEngine,
	prometheusSecret corev1.Secret,
	inputHash string,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info(fmt.Sprintf("Defining WatcherDecisionEngine deployment '%s'", instance.Name))

	// If the prometheus config is providing CA cert a volume must be mounted
	prometheusCaCertSecret := string(prometheusSecret.Data[PrometheusCaCertSecret])
	prometheusCaCertKey := string(prometheusSecret.Data[PrometheusCaCertKey])
	prometheusCaCert := make(map[string]string)
	if prometheusCaCertSecret != "" && prometheusCaCertKey != "" {
		prometheusCaCert = map[string]string{
			"casecret_key":  prometheusCaCertKey,
			"casecret_name": prometheusCaCertSecret,
		}

	}

	ss := statefulset.NewStatefulSet(watcherdecisionengine.StatefulSet(instance, inputHash, prometheusCaCert, getDecisionEngineServiceLabels()), r.RequeueTimeout)

	ctrlResult, err := ss.CreateOrPatch(ctx, h)
	if err != nil && !k8s_errors.IsNotFound(err) {
		Log.Error(err, "Deployment failed")
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DeploymentReadyErrorMessage,
			err.Error()))
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{} || k8s_errors.IsNotFound(err)) {
		Log.Info("Deployment in progress")
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DeploymentReadyRunningMessage))
		// It is OK to return success as we are watching for StatefulSet changes
		return ctrlResult, nil
	}

	statefulSet := ss.GetStatefulSet()
	if statefulSet.Generation == statefulSet.Status.ObservedGeneration {
		instance.Status.ReadyCount = statefulSet.Status.ReadyReplicas
	}

	if instance.Status.ReadyCount == *instance.Spec.Replicas && statefulSet.Generation == statefulSet.Status.ObservedGeneration {
		Log.Info("Deployment is ready")
		instance.Status.Conditions.MarkTrue(condition.DeploymentReadyCondition, condition.DeploymentReadyMessage)
	} else {
		Log.Info("Deployment is not ready", "Status", ss.GetStatefulSet().Status)
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DeploymentReadyRunningMessage))
		// It is OK to return success as we are watching for StatefulSet changes
		return ctrl.Result{}, nil
	}
	return ctrl.Result{}, nil
}

func (r *WatcherDecisionEngineReconciler) reconcileDelete(ctx context.Context, instance *watcherv1beta1.WatcherDecisionEngine, helper *helper.Helper) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info(fmt.Sprintf("Reconcile Service '%s' delete started", instance.Name))

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

	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
	Log.Info(fmt.Sprintf("Reconciled Service '%s' delete successfully", instance.Name))
	return ctrl.Result{}, nil
}

func (r *WatcherDecisionEngineReconciler) createHashOfInputHashes(
	ctx context.Context,
	instance *watcherv1beta1.WatcherDecisionEngine,
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

func getDecisionEngineServiceLabels() map[string]string {
	return map[string]string{
		common.AppSelector: WatcherDecisionEngineLabelPrefix,
	}
}
