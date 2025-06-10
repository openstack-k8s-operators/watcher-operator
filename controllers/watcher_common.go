package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	passwordSecretField     = ".spec.secret"
	prometheusSecretField   = ".spec.prometheusSecret"
	caBundleSecretNameField = ".spec.tls.caBundleSecretName"
	tlsAPIInternalField     = ".spec.tls.api.internal.secretName"
	tlsAPIPublicField       = ".spec.tls.api.public.secretName"
	topologyField           = ".spec.topologyRef.Name"
)

var (
	apiWatchFields = []string{
		passwordSecretField,
		prometheusSecretField,
		tlsAPIInternalField,
		tlsAPIPublicField,
		caBundleSecretNameField,
		topologyField,
	}
	applierWatchFields = []string{
		passwordSecretField,
		prometheusSecretField,
		caBundleSecretNameField,
		topologyField,
	}
	watcherWatchFields = []string{
		passwordSecretField,
		prometheusSecretField,
	}
	decisionEngineWatchFields = []string{
		passwordSecretField,
		prometheusSecretField,
		caBundleSecretNameField,
		topologyField,
	}
)

const (
	// TransportURLSelector is the name of key in the secret created by TransportURL
	TransportURLSelector = "transport_url"
	// DatabaseAccount is the name of key in the secret for the name of the Database Acount object
	DatabaseAccount = "database_account"
	// DatabaseUsername is the name of key in the secret for the user name used to login to the database
	DatabaseUsername = "database_username"
	// DatabaseUsername is the name of key in the secret for the password used to login to the database
	DatabasePassword = "database_password"
	// DatabaseUsername is the name of key in the secret for the database
	// hostname
	DatabaseHostname = "database_hostname"
	// Prometheus configuration keys in prometheusSecret
	PrometheusHost         = "host"
	PrometheusPort         = "port"
	PrometheusCaCertSecret = "ca_secret"
	PrometheusCaCertKey    = "ca_key"

	// WatcherAPILabelPrefix - a unique, service binary specific prefix for the
	// labels the WatcherAPI controller uses on children objects
	WatcherAPILabelPrefix = "watcher-api"
	// WatcherApplierLabelPrefix - a unique, service binary specific prefix for the
	// labels the WatcherApplier controller uses on children objects
	WatcherApplierLabelPrefix = "watcher-applier"
	// WatcherDecisionEngineLabelPrefix - a unique, service binary specific prefix
	// for the labels the WatcherDecisionEngine controller uses on child objects
	WatcherDecisionEngineLabelPrefix = "watcher-decision-engine"
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

type conditionUpdater interface {
	Set(c *condition.Condition)
	MarkTrue(t condition.Type, messageFormat string, messageArgs ...interface{})
}

// ensureSecret - ensures that the Secret object exists and the expected fields
// are in the Secret. It returns a hash of the values of the expected fields.
func ensureSecret(
	ctx context.Context,
	secretName types.NamespacedName,
	expectedFields []string,
	reader client.Reader,
	conditionUpdater conditionUpdater,
	requeueTimeout time.Duration,
) (string, ctrl.Result, corev1.Secret, error) {
	secret := &corev1.Secret{}
	err := reader.Get(ctx, secretName, secret)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			log.FromContext(ctx).Info(fmt.Sprintf("secret %s not found", secretName))
			conditionUpdater.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.InputReadyWaitingMessage))
			return "",
				ctrl.Result{RequeueAfter: requeueTimeout},
				*secret,
				nil
		}
		conditionUpdater.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.InputReadyErrorMessage,
			err.Error()))
		return "", ctrl.Result{}, *secret, err
	}

	// collect the secret values the caller expects to exist
	values := [][]byte{}
	for _, field := range expectedFields {
		val, ok := secret.Data[field]
		if !ok {
			err := fmt.Errorf("field '%s' not found in secret/%s", field, secretName.Name)
			conditionUpdater.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.InputReadyErrorMessage,
				err.Error()))
			return "", ctrl.Result{}, *secret, err
		}
		values = append(values, val)
	}

	hash, err := util.ObjectHash(values)
	if err != nil {
		conditionUpdater.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.InputReadyErrorMessage,
			err.Error()))
		return "", ctrl.Result{}, *secret, err
	}

	return hash, ctrl.Result{}, *secret, nil
}

func GenerateConfigsGeneric(
	ctx context.Context, helper *helper.Helper,
	instance client.Object,
	envVars *map[string]env.Setter,
	templateParameters map[string]interface{},
	customData map[string]string,
	cmLabels map[string]string,
	scripts bool,
) error {

	extraTemplates := map[string]string{
		"00-default.conf":    "/00-default.conf",
		"watcher-blank.conf": "/watcher-blank.conf",
	}

	cms := []util.Template{
		// Templates where the watcher config is stored
		{
			Name:               fmt.Sprintf("%s-config-data", instance.GetName()),
			Namespace:          instance.GetNamespace(),
			Type:               util.TemplateTypeConfig,
			InstanceType:       instance.GetObjectKind().GroupVersionKind().Kind,
			ConfigOptions:      templateParameters,
			CustomData:         customData,
			Labels:             cmLabels,
			AdditionalTemplate: extraTemplates,
		},
	}
	if scripts {
		cms = append(cms, util.Template{
			Name:          fmt.Sprintf("%s-scripts", instance.GetName()),
			Namespace:     instance.GetNamespace(),
			Type:          util.TemplateTypeScripts,
			InstanceType:  instance.GetObjectKind().GroupVersionKind().Kind,
			ConfigOptions: templateParameters,
			Labels:        cmLabels,
		})
	}
	return secret.EnsureSecrets(ctx, helper, instance, cms, envVars)
}

// ensureMemcached - gets the Memcached instance used for watcher services cache backend
func ensureMemcached(
	ctx context.Context,
	helper *helper.Helper,
	namespaceName string,
	memcachedName string,
	conditionUpdater conditionUpdater,
) (*memcachedv1.Memcached, error) {
	memcached, err := memcachedv1.GetMemcachedByName(ctx, helper, memcachedName, namespaceName)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			conditionUpdater.Set(condition.FalseCondition(
				condition.MemcachedReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.MemcachedReadyWaitingMessage))
			return nil, fmt.Errorf("memcached %s not found", memcachedName)
		}
		conditionUpdater.Set(condition.FalseCondition(
			condition.MemcachedReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.MemcachedReadyErrorMessage,
			err.Error()))
		return nil, err
	}

	if !memcached.IsReady() {
		conditionUpdater.Set(condition.FalseCondition(
			condition.MemcachedReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.MemcachedReadyWaitingMessage))
		return nil, fmt.Errorf("memcached %s is not ready", memcachedName)
	}
	conditionUpdater.MarkTrue(condition.MemcachedReadyCondition, condition.MemcachedReadyMessage)

	return memcached, err
}

func getAPIServiceLabels() map[string]string {
	return map[string]string{
		common.AppSelector: WatcherAPILabelPrefix,
	}
}

type topologyHandler interface {
	GetSpecTopologyRef() *topologyv1.TopoRef
	GetLastAppliedTopology() *topologyv1.TopoRef
	SetLastAppliedTopology(t *topologyv1.TopoRef)
}

// GetLastAppliedTopologyRef - Returns a TopoRef object that can be passed to the
// Handle topology logic
func GetLastAppliedTopologyRef(t topologyHandler, ns string) *topologyv1.TopoRef {
	lastAppliedTopologyName := ""
	if l := t.GetLastAppliedTopology(); l != nil {
		lastAppliedTopologyName = l.Name
	}
	return &topologyv1.TopoRef{
		Name:      lastAppliedTopologyName,
		Namespace: ns,
	}
}

// ensureTopology - when a Topology CR is referenced, remove the
// finalizer from a previous referenced Topology (if any), and retrieve the
// newly referenced topology object
func ensureTopology(
	ctx context.Context,
	helper *helper.Helper,
	instance topologyHandler,
	finalizer string,
	conditionUpdater conditionUpdater,
	defaultLabelSelector metav1.LabelSelector,
) (*topologyv1.Topology, error) {

	topology, err := topologyv1.EnsureServiceTopology(
		ctx,
		helper,
		instance.GetSpecTopologyRef(),
		GetLastAppliedTopologyRef(instance, helper.GetBefore().GetNamespace()),
		finalizer,
		defaultLabelSelector,
	)
	if err != nil {
		conditionUpdater.Set(condition.FalseCondition(
			condition.TopologyReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.TopologyReadyErrorMessage,
			err.Error()))
		return nil, fmt.Errorf("waiting for Topology requirements: %w", err)
	}
	// update the Status with the last retrieved Topology (or set it to nil)
	instance.SetLastAppliedTopology(instance.GetSpecTopologyRef())
	// update the Topology condition only when a Topology is referenced and has
	// been retrieved (err == nil)
	if tr := instance.GetSpecTopologyRef(); tr != nil {
		// update the TopologyRef associated condition
		conditionUpdater.MarkTrue(
			condition.TopologyReadyCondition,
			condition.TopologyReadyMessage,
		)
	}
	return topology, nil
}
