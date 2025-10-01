// Package controllers contains the Kubernetes controllers for managing Watcher components
package controllers

import (
	"context"
	"errors"
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
	caBundleSecretNameField = ".spec.tls.caBundleSecretName" //nolint:gosec // G101: Not actual credentials, just field path
	tlsAPIInternalField     = ".spec.tls.api.internal.secretName"
	tlsAPIPublicField       = ".spec.tls.api.public.secretName"
	topologyField           = ".spec.topologyRef.Name"
	memcachedInstanceField  = ".spec.memcachedInstance"
)

var (
	apiWatchFields = []string{
		passwordSecretField,
		prometheusSecretField,
		tlsAPIInternalField,
		tlsAPIPublicField,
		caBundleSecretNameField,
		topologyField,
		memcachedInstanceField,
	}
	applierWatchFields = []string{
		passwordSecretField,
		prometheusSecretField,
		caBundleSecretNameField,
		topologyField,
		memcachedInstanceField,
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
		memcachedInstanceField,
	}
)

const (
	// TransportURLSelector is the name of key in the secret created by TransportURL
	TransportURLSelector = "transport_url"
	// NotificationURLSelector is the name of key in the secret created by the notification TransportURL
	NotificationURLSelector = "notification_url"
	// QuorumQueuesSelector is the name of key in the secret created by TransportURL for quorum queues enablement
	QuorumQueuesSelector = "quorumqueues"
	// DatabaseAccount is the name of key in the secret for the name of the Database Acount object
	DatabaseAccount = "database_account"
	// DatabaseUsername is the name of key in the secret for the user name used to login to the database
	DatabaseUsername = "database_username"
	// DatabasePassword is the name of key in the secret for the password used to login to the database
	DatabasePassword = "database_password"
	// DatabaseHostname is the name of key in the secret for the database hostname
	DatabaseHostname = "database_hostname"
	// PrometheusHost is the key for the Prometheus host in prometheusSecret
	PrometheusHost = "host"
	// PrometheusPort is the key for the Prometheus port in prometheusSecret
	PrometheusPort = "port"
	// PrometheusCaCertSecret is the key for the Prometheus CA certificate secret in prometheusSecret
	PrometheusCaCertSecret = "ca_secret"
	// PrometheusCaCertKey is the key for the Prometheus CA certificate key in prometheusSecret
	PrometheusCaCertKey = "ca_key"

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

// Static error variables for err113 compliance
var (
	// ErrSecretFieldNotFound indicates that a required field was not found in a secret
	ErrSecretFieldNotFound = errors.New("field not found in secret")

	// ErrMemcachedNotFound indicates that the memcached instance was not found
	ErrMemcachedNotFound = errors.New("memcached not found")

	// ErrMemcachedNotReady indicates that the memcached instance is not ready
	ErrMemcachedNotReady = errors.New("memcached is not ready")

	// ErrRetrievingSecretData indicates an error retrieving required data from a secret
	ErrRetrievingSecretData = errors.New("error retrieving required data from secret")

	// ErrRetrievingNotificationURLSecretData indicates an error retrieving required data from notificationURL secret
	ErrRetrievingNotificationURLSecretData = errors.New("error retrieving required data from notificationURL secret")

	// ErrRetrievingTransportURLSecretData indicates an error retrieving required data from transporturl secret
	ErrRetrievingTransportURLSecretData = errors.New("error retrieving required data from transporturl secret")

	// ErrRetrievingPrometheusSecretData indicates an error retrieving required data from prometheus secret
	ErrRetrievingPrometheusSecretData = errors.New("error retrieving required data from prometheus secret")

	// ErrTransportURLFieldMissing indicates that the TransportURL secret does not have the 'transport_url' field
	ErrTransportURLFieldMissing = errors.New("the TransportURL secret does not have 'transport_url' field")
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
	MarkTrue(t condition.Type, messageFormat string, messageArgs ...any)
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
			// Since secrets should have been manually created by the user and referenced in the spec,
			// we treat this as a warning because it means that the service will not be able to start.
			log.FromContext(ctx).Info(fmt.Sprintf("secret %s not found", secretName))
			conditionUpdater.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
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
			err := fmt.Errorf("%w: '%s' in secret/%s", ErrSecretFieldNotFound, field, secretName.Name)
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

// GenerateConfigsGeneric generates configuration files for watcher components
func GenerateConfigsGeneric(
	ctx context.Context, helper *helper.Helper,
	instance client.Object,
	envVars *map[string]env.Setter,
	templateParameters map[string]any,
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
			// Memcached should be automatically created by the encompassing OpenStackControlPlane,
			// but we don't propagate its name into the "memcachedInstance" field of other sub-resources,
			// so if it is missing at this point, it *could* be because there's a mismatch between the
			// name of the Memcached CR and the name of the Memcached instance referenced by this CR.
			// Since that situation would block further reconciliation, we treat it as a warning.
			conditionUpdater.Set(condition.FalseCondition(
				condition.MemcachedReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.MemcachedReadyWaitingMessage))
			return nil, fmt.Errorf("%w: %s", ErrMemcachedNotFound, memcachedName)
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
		return nil, fmt.Errorf("%w: %s", ErrMemcachedNotReady, memcachedName)
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
