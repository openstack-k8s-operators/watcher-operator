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
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"

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
	ctrlResult, err = db.WaitForDBCreated(ctx, h)
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
		Complete(r)
}
