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
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	rabbitmqv1 "github.com/openstack-k8s-operators/infra-operator/apis/rabbitmq/v1beta1"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	watcherv1beta1 "github.com/openstack-k8s-operators/watcher-operator/api/v1beta1"

	"github.com/openstack-k8s-operators/watcher-operator/pkg/watcher"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
)

// WatcherReconciler reconciles a Watcher object
type WatcherReconciler struct {
	ReconcilerBase
}

// GetLogger returns a logger object with a prefix of "controller.name" and additional controller context fields
func (r *WatcherReconciler) GetLogger(ctx context.Context) logr.Logger {
	return log.FromContext(ctx).WithName("Controllers").WithName("Watcher")
}

//+kubebuilder:rbac:groups=watcher.openstack.org,resources=watchers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=watcher.openstack.org,resources=watchers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=watcher.openstack.org,resources=watchers/finalizers,verbs=update
//+kubebuilder:rbac:groups=watcher.openstack.org,resources=watcherapis,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=watcher.openstack.org,resources=watcherapis/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=watcher.openstack.org,resources=watcherapis/finalizers,verbs=update
//+kubebuilder:rbac:groups=watcher.openstack.org,resources=watcherdecisionengines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=watcher.openstack.org,resources=watcherdecisionengines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=watcher.openstack.org,resources=watcherdecisionengines/finalizers,verbs=update
//+kubebuilder:rbac:groups=watcher.openstack.org,resources=watcherappliers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=watcher.openstack.org,resources=watcherappliers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=watcher.openstack.org,resources=watcherappliers/finalizers,verbs=update
//+kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbaccounts/finalizers,verbs=update
//+kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbdatabases,verbs=get;list;watch;create;update;patch;delete;
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete;
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete;
//+kubebuilder:rbac:groups=rabbitmq.openstack.org,resources=transporturls,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneservices,verbs=get;list;watch;create;update;patch;delete;
//+kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneendpoints,verbs=get;list;watch;create;update;patch;delete;

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *WatcherReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	Log := r.GetLogger(ctx)

	instance := &watcherv1beta1.Watcher{}
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

	Log.Info(fmt.Sprintf("Reconciling Watcher instance '%s'", instance.Name))

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

	serviceLabels := map[string]string{
		common.AppSelector: watcher.ServiceName,
	}

	_ = serviceLabels

	// Save a copy of the conditions so that we can restore the LastTransitionTime
	// when a condition's state doesn't change.
	isNewInstance := instance.Status.Conditions == nil
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

	// initialize the status
	err = r.initStatus(instance)
	if err != nil {
		return ctrl.Result{}, err
	}

	// If we're not deleting this and the service object doesn't have our finalizer, add it.
	if instance.DeletionTimestamp.IsZero() && controllerutil.AddFinalizer(instance, helper.GetFinalizer()) || isNewInstance {
		return ctrl.Result{}, nil
	}

	// Handle service delete
	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance, helper)
	}

	//
	// Create the DB and required DB account
	//
	db, result, err := r.ensureDB(ctx, helper, instance)
	if err != nil {
		return ctrl.Result{}, err
	} else if (result != ctrl.Result{}) {
		return result, nil
	}
	_ = db
	// create service DB - end

	//
	// create RabbitMQ transportURL CR and get the actual URL from the associated secret that is created
	// not-ready condition is managed here instead of in ensureMQ to distinguish between Error (when receiving)
	// an error, or Running when transportURL is empty.
	//
	transportURL, op, err := r.ensureMQ(ctx, instance, helper, serviceLabels)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			watcherv1beta1.WatcherRabbitMQTransportURLReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			watcherv1beta1.WatcherRabbitMQTransportURLReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	if transportURL == nil {
		Log.Info(fmt.Sprintf("Waiting for TransportURL for %s to be created", instance.Name))
		instance.Status.Conditions.Set(condition.FalseCondition(
			watcherv1beta1.WatcherRabbitMQTransportURLReadyCondition,
			condition.RequestedReason,
			condition.SeverityWarning,
			watcherv1beta1.WatcherRabbitMQTransportURLReadyRunningMessage))
		return ctrl.Result{RequeueAfter: time.Duration(10) * time.Second}, nil
	}

	_ = op
	// end of TransportURL creation

	// Check we have the required inputs
	hash, _, _, err := ensureSecret(
		ctx,
		types.NamespacedName{Namespace: instance.Namespace, Name: instance.Spec.Secret},
		[]string{
			instance.Spec.PasswordSelectors.Service,
		},
		helper.GetClient(),
		&instance.Status.Conditions,
		r.RequeueTimeout,
	)
	if err != nil || hash == "" {
		// Empty hash means that there is some problem retrieving the key from the secret
		return ctrl.Result{}, errors.New("error retrieving required data from secret")
	}

	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)
	// End of Input Ready check

	// Create Keystone Service creation. The endpoint will be created by WatcherAPI
	_, err = r.ensureKeystoneSvc(ctx, helper, instance, serviceLabels)

	if err != nil {

		return ctrl.Result{}, err
	}

	// End of Keystone service creation

	// remove finalizers from unused MariaDBAccount records
	// this assumes all database-depedendent deployments are up and
	// running with current database account info
	err = mariadbv1.DeleteUnusedMariaDBAccountFinalizers(
		ctx, helper, watcher.DatabaseCRName,
		instance.Spec.DatabaseAccount, instance.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	// We reached the end of the Reconcile, update the Ready condition based on
	// the sub conditions
	if instance.Status.Conditions.AllSubConditionIsTrue() {
		instance.Status.Conditions.MarkTrue(
			condition.ReadyCondition, condition.ReadyMessage)
	}

	return ctrl.Result{}, nil
}

// Initialize Watcher status
func (r *WatcherReconciler) initStatus(instance *watcherv1beta1.Watcher) error {

	err := r.initConditions(instance)
	if err != nil {
		return err
	}

	// Update the lastObserved generation before evaluating conditions
	instance.Status.ObservedGeneration = instance.Generation

	return nil
}

// Initialize Watcher conditions
func (r *WatcherReconciler) initConditions(instance *watcherv1beta1.Watcher) error {
	if instance.Status.Conditions == nil {
		instance.Status.Conditions = condition.Conditions{}
	}
	// initialize conditions used later as Status=Unknown
	cl := condition.CreateList(
		// Mark ReadyCondition as Unknown from the beginning, because the
		// Reconcile function is in progress. If this condition is not marked
		// as True and is still in the "Unknown" state, we `Mirror(` the actual
		// failure/in-progress operation
		condition.UnknownCondition(condition.ReadyCondition, condition.InitReason, condition.ReadyInitMessage),
		condition.UnknownCondition(condition.DBReadyCondition, condition.InitReason, condition.DBReadyInitMessage),
		condition.UnknownCondition(
			watcherv1beta1.WatcherRabbitMQTransportURLReadyCondition,
			condition.InitReason,
			condition.RabbitMqTransportURLReadyInitMessage),
		condition.UnknownCondition(
			condition.InputReadyCondition,
			condition.InitReason,
			condition.InputReadyInitMessage),
		condition.UnknownCondition(
			condition.KeystoneServiceReadyCondition,
			condition.InitReason,
			"Service registration not started"),
	)

	instance.Status.Conditions.Init(&cl)

	return nil
}

// ensureDB creates the require DB in a running mariadb instance
func (r *WatcherReconciler) ensureDB(
	ctx context.Context,
	h *helper.Helper,
	instance *watcherv1beta1.Watcher,
) (*mariadbv1.Database, ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info(fmt.Sprintf("Reconciling the DB instance for '%s'", instance.Name))

	// ensure MariaDBAccount exists without being yet associated with any database.
	// This account record may be created by the openstack-operator up front.
	_, _, err := mariadbv1.EnsureMariaDBAccount(
		ctx, h, instance.Spec.DatabaseAccount,
		instance.Namespace, false, watcher.DatabaseUsernamePrefix,
	)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			mariadbv1.MariaDBAccountReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			mariadbv1.MariaDBAccountNotReadyMessage,
			err.Error()))

		return nil, ctrl.Result{}, err
	}
	instance.Status.Conditions.MarkTrue(
		mariadbv1.MariaDBAccountReadyCondition,
		mariadbv1.MariaDBAccountReadyMessage)

	//
	// create watcher DB instance
	//
	db := mariadbv1.NewDatabaseForAccount(
		instance.Spec.DatabaseInstance, // mariadb/galera service to target
		watcher.DatabaseName,           // name used in CREATE DATABASE in mariadb
		watcher.DatabaseCRName,         // CR name for MariaDBDatabase
		instance.Spec.DatabaseAccount,  // CR name for MariaDBAccount
		instance.Namespace,             // namespace
	)

	// create or patch the DB
	ctrlResult, err := db.CreateOrPatchAll(ctx, h)

	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DBReadyErrorMessage,
			err.Error()))
		return db, ctrl.Result{}, err
	}
	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DBReadyRunningMessage))
		return db, ctrlResult, nil
	}
	// wait for the DB to be setup
	ctrlResult, err = db.WaitForDBCreatedWithTimeout(ctx, h, r.RequeueTimeout)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DBReadyErrorMessage,
			err.Error()))
		return db, ctrlResult, err
	}
	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DBReadyRunningMessage))
		return db, ctrlResult, nil
	}

	instance.Status.Conditions.MarkTrue(condition.DBReadyCondition, condition.DBReadyMessage)

	return db, ctrl.Result{}, err
}

// Create the required RabbitMQ
func (r *WatcherReconciler) ensureMQ(
	ctx context.Context,
	instance *watcherv1beta1.Watcher,
	h *helper.Helper,
	serviceLabels map[string]string,
) (*rabbitmqv1.TransportURL, controllerutil.OperationResult, error) {
	Log := r.GetLogger(ctx)
	Log.Info(fmt.Sprintf("Reconciling the RabbitMQ TransportURL for '%s'", instance.Name))

	transportURL := &rabbitmqv1.TransportURL{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-watcher-transport", instance.Name),
			Namespace: instance.Namespace,
			Labels:    serviceLabels,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, transportURL, func() error {
		transportURL.Spec.RabbitmqClusterName = instance.Spec.RabbitMqClusterName

		err := controllerutil.SetControllerReference(instance, transportURL, r.Scheme)
		return err
	})

	if err != nil && !k8s_errors.IsNotFound(err) {
		return nil, op, util.WrapErrorForObject(
			fmt.Sprintf("Error create or update TransportURL object %s-watcher-transport", instance.Name),
			transportURL,
			err,
		)
	}

	if op != controllerutil.OperationResultNone {
		Log.Info(fmt.Sprintf("TransportURL %s successfully reconciled - operation: %s", transportURL.Name, string(op)))
	}

	// If transportURL is not ready, it returns nil
	if !transportURL.IsReady() || transportURL.Status.SecretName == "" {
		Log.Info(fmt.Sprintf("Waiting for TransportURL %s secret to be created", transportURL.Name))
		return nil, op, nil
	}

	secretName := types.NamespacedName{Namespace: instance.Namespace, Name: transportURL.Status.SecretName}
	secret := &corev1.Secret{}
	err = h.GetClient().Get(ctx, secretName, secret)
	if err != nil {
		return nil, op, err
	}

	_, ok := secret.Data[TransportURLSelector]
	if !ok {
		return nil, op, fmt.Errorf(
			"the TransportURL secret %s does not have 'transport_url' field", transportURL.Status.SecretName)
	}

	instance.Status.Conditions.MarkTrue(watcherv1beta1.WatcherRabbitMQTransportURLReadyCondition, watcherv1beta1.WatcherRabbitMQTransportURLReadyMessage)
	return transportURL, op, nil
}

func (r *WatcherReconciler) ensureKeystoneSvc(
	ctx context.Context,
	h *helper.Helper,
	instance *watcherv1beta1.Watcher,
	serviceLabels map[string]string,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info(fmt.Sprintf("Reconciling the Keystone Service for '%s'", instance.Name))

	//
	// create Keystone service and user
	//

	ksSvcSpec := keystonev1.KeystoneServiceSpec{
		ServiceType:        watcher.ServiceType,
		ServiceName:        watcher.ServiceName,
		ServiceDescription: "Watcher Service",
		Enabled:            true,
		ServiceUser:        instance.Spec.ServiceUser,
		Secret:             instance.Spec.Secret,
		PasswordSelector:   instance.Spec.PasswordSelectors.Service,
	}

	ksSvc := keystonev1.NewKeystoneService(ksSvcSpec, instance.Namespace, serviceLabels, time.Duration(10)*time.Second)
	ctrlResult, err := ksSvc.CreateOrPatch(ctx, h)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.KeystoneServiceReadyCondition,
			condition.CreationFailedReason,
			condition.SeverityError,
			"Error while creating Keystone Service for Watcher"))
		return ctrlResult, err
	}

	// mirror the Status, Reason, Severity and Message of the latest keystoneservice condition
	// into a local condition with the type condition.KeystoneServiceReadyCondition
	c := ksSvc.GetConditions().Mirror(condition.KeystoneServiceReadyCondition)
	if c != nil {
		instance.Status.Conditions.Set(c)
	}

	if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	instance.Status.ServiceID = ksSvc.GetServiceID()

	//if instance.Status.Hash == nil {
	//	instance.Status.Hash = map[string]string{}
	//}

	return ctrlResult, nil
}

func (r *WatcherReconciler) reconcileDelete(ctx context.Context, instance *watcherv1beta1.Watcher, helper *helper.Helper) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info(fmt.Sprintf("Reconcile Service '%s' delete started", instance.Name))

	// remove db finalizer first
	db, err := mariadbv1.GetDatabaseByNameAndAccount(ctx, helper, watcher.DatabaseCRName, instance.Spec.DatabaseAccount, instance.Namespace)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	if !k8s_errors.IsNotFound(err) {
		if err := db.DeleteFinalizer(ctx, helper); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Remove the finalizer from our KeystoneService CR
	keystoneService, err := keystonev1.GetKeystoneServiceWithName(ctx, helper, watcher.ServiceName, instance.Namespace)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	if err == nil {
		if controllerutil.RemoveFinalizer(keystoneService, helper.GetFinalizer()) {
			err = helper.GetClient().Update(ctx, keystoneService)
			if err != nil && !k8s_errors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			util.LogForObject(helper, "Removed finalizer from our KeystoneService", instance)
		}
	}

	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
	Log.Info(fmt.Sprintf("Reconciled Service '%s' delete successfully", instance.Name))
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WatcherReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&watcherv1beta1.Watcher{}).
		Owns(&watcherv1beta1.WatcherAPI{}).
		Owns(&watcherv1beta1.WatcherDecisionEngine{}).
		Owns(&watcherv1beta1.WatcherApplier{}).
		Owns(&mariadbv1.MariaDBDatabase{}).
		Owns(&mariadbv1.MariaDBAccount{}).
		Owns(&rabbitmqv1.TransportURL{}).
		Owns(&keystonev1.KeystoneService{}).
		Complete(r)
}
