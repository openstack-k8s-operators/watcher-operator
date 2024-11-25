package controllers

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// GetLogger returns a logger object with a prefix of "controller.name" and additional controller context fields
func (r *ReconcilerBase) GetLogger(ctx context.Context) logr.Logger {
	return log.FromContext(ctx).WithName("Controllers").WithName("ReconcilerBase")
}

// ReconcilerBase provides a common set of clients scheme and loggers for all reconcilers.
type ReconcilerBase struct {
	Client         client.Client
	Kclient        kubernetes.Interface
	Scheme         *runtime.Scheme
	RequeueTimeout time.Duration
}

// Manageable all types that conform to this interface can be setup with a controller-runtime manager.
type Manageable interface {
	SetupWithManager(mgr ctrl.Manager) error
}

// Reconciler represents a generic interface for all Reconciler objects in watcher
type Reconciler interface {
	Manageable
	SetRequeueTimeout(timeout time.Duration)
}

// NewReconcilerBase constructs a ReconcilerBase given a manager and Kclient.
func NewReconcilerBase(
	mgr ctrl.Manager, kclient kubernetes.Interface,
) ReconcilerBase {
	return ReconcilerBase{
		Client:         mgr.GetClient(),
		Scheme:         mgr.GetScheme(),
		Kclient:        kclient,
		RequeueTimeout: time.Duration(5) * time.Second,
	}
}

// SetRequeueTimeout overrides the default RequeueTimeout of the Reconciler
func (r *ReconcilerBase) SetRequeueTimeout(timeout time.Duration) {
	r.RequeueTimeout = timeout
}

// Reconcilers holds all the Reconciler objects of the watcher-operator to
// allow generic management of them.
type Reconcilers struct {
	reconcilers map[string]Reconciler
}

// NewReconcilers constructs all watcher Reconciler objects
func NewReconcilers(mgr ctrl.Manager, kclient *kubernetes.Clientset) *Reconcilers {
	return &Reconcilers{
		reconcilers: map[string]Reconciler{
			"Watcher": &WatcherReconciler{
				ReconcilerBase: NewReconcilerBase(mgr, kclient),
			},
			"WatcherAPI": &WatcherAPIReconciler{
				ReconcilerBase: NewReconcilerBase(mgr, kclient),
			},
			"WatcherDecisionEngine": &WatcherDecisionEngineReconciler{
				ReconcilerBase: NewReconcilerBase(mgr, kclient),
			},
			"WatcherApplier": &WatcherApplierReconciler{
				ReconcilerBase: NewReconcilerBase(mgr, kclient),
			},
		}}
}

// Setup starts the reconcilers by connecting them to the Manager
func (r *Reconcilers) Setup(mgr ctrl.Manager, setupLog logr.Logger) error {
	var err error
	for name, controller := range r.reconcilers {
		if err = controller.SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", name)
			return err
		}
	}
	return nil
}

// OverrideRequeueTimeout overrides the default RequeueTimeout of our reconcilers
func (r *Reconcilers) OverrideRequeueTimeout(timeout time.Duration) {
	for _, reconciler := range r.reconcilers {
		reconciler.SetRequeueTimeout(timeout)
	}
}
