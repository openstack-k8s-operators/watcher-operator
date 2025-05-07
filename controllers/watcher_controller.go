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
	routev1 "github.com/openshift/api/route/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rabbitmqv1 "github.com/openstack-k8s-operators/infra-operator/apis/rabbitmq/v1beta1"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/cronjob"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/job"
	"github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	common_rbac "github.com/openstack-k8s-operators/lib-common/modules/common/rbac"
	"github.com/openstack-k8s-operators/lib-common/modules/common/route"
	"github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	watcherv1beta1 "github.com/openstack-k8s-operators/watcher-operator/api/v1beta1"

	"github.com/openstack-k8s-operators/watcher-operator/pkg/watcher"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
)

// jgilaber helper types to expose services with TLS, copied from
// openstack-operator, should be removed once watcher-operator is integrated
// ServiceTLSDetails - tls settings for the endpoint
type ServiceTLSDetails struct {
	Enabled  bool
	CertName string
	tls.GenericService
	tls.Ca
}

// ServiceDetails - service details
type ServiceDetails struct {
	Spec         *k8s_corev1.Service
	OverrideSpec service.RoutedOverrideSpec
	TLS          ServiceTLSDetails
}

// RouteDetails - route details
type RouteDetails struct {
	Create       bool
	Route        *routev1.Route
	OverrideSpec route.OverrideSpec
	TLS          RouteTLSDetails
}

// RouteTLSDetails - tls settings for the endpoint
type RouteTLSDetails struct {
	Enabled    bool
	SecretName *string
	CertName   string
	IssuerName string
	tls.Ca
}

// EndpointDetail - endpoint details
type EndpointDetail struct {
	Name        string
	Namespace   string
	Type        service.Endpoint
	Annotations map[string]string
	Labels      map[string]string
	Service     ServiceDetails
	Route       RouteDetails
	Hostname    *string
	Proto       service.Protocol
	EndpointURL string
}

func (ed *EndpointDetail) ensureRoute(
	ctx context.Context,
	helper *helper.Helper,
	instance *watcherv1beta1.Watcher,
	svc *k8s_corev1.Service,
) (ctrl.Result, error) {
	// check if there is already a route with watcherapi labels
	routes, err := GetRoutesListWithLabel(ctx, helper, instance.Namespace, getAPIServiceLabels())
	if err != nil {
		return ctrl.Result{}, err
	}
	for _, route := range routes.Items {
		instanceRef := metav1.OwnerReference{
			APIVersion:         instance.APIVersion,
			Kind:               instance.Kind,
			Name:               instance.GetName(),
			UID:                instance.GetUID(),
			BlockOwnerDeletion: ptr.To(true),
			Controller:         ptr.To(true),
		}

		owner := metav1.GetControllerOf(&route.ObjectMeta)

		// Delete the route if the service was changed not to expose a route
		if svc.ObjectMeta.Annotations[service.AnnotationIngressCreateKey] == "false" &&
			route.Spec.To.Name == ed.Name &&
			owner != nil && owner.UID == instance.GetUID() {
			// Delete any other owner refs from ref list to not block deletion until owners are gone
			route.SetOwnerReferences([]metav1.OwnerReference{instanceRef})

			// Delete route
			err := helper.GetClient().Delete(ctx, &route)
			if err != nil && !k8s_errors.IsNotFound(err) {
				err = fmt.Errorf("Error deleting route %s: %w", route.Name, err)
				return ctrl.Result{}, err
			}

			if ed.Service.OverrideSpec.EndpointURL != nil {
				ed.Service.OverrideSpec.EndpointURL = nil
				helper.GetLogger().Info(fmt.Sprintf("Service %s override endpointURL removed", svc.Name))
			}
		}

	}
	if ed.Route.Create {
		if ed.Service.OverrideSpec.EmbeddedLabelsAnnotations == nil {
			ed.Service.OverrideSpec.EmbeddedLabelsAnnotations = &service.EmbeddedLabelsAnnotations{}
		}
		ed.Labels = getAPIServiceLabels()

		ctrlResult, err := ed.CreateRoute(ctx, helper)
		return ctrlResult, err
	}

	return ctrl.Result{}, nil
}

func (ed *EndpointDetail) CreateRoute(
	ctx context.Context,
	helper *helper.Helper,
) (ctrl.Result, error) {
	// initialize the route with any custom provided route override
	// per default use the service name as targetPortName if we don't have the annotation.
	targetPortName := ed.Service.Spec.Name
	if name, ok := ed.Service.Spec.ObjectMeta.Annotations[service.AnnotationIngressTargetPortNameKey]; ok && name != "" {
		targetPortName = name
	}
	endptRoute, err := route.NewRoute(
		route.GenericRoute(&route.GenericRouteDetails{
			Name:           ed.Name,
			Namespace:      ed.Namespace,
			Labels:         ed.Labels,
			ServiceName:    ed.Service.Spec.Name,
			TargetPortName: targetPortName,
		}),
		time.Duration(5)*time.Second,
		[]route.OverrideSpec{ed.Route.OverrideSpec},
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	// if route TLS is disabled -> create the route
	// if TLS is enabled and the route does not yet exist -> create the route
	// to get the hostname for creating the cert
	serviceRoute := &routev1.Route{}
	err = helper.GetClient().Get(ctx, types.NamespacedName{Name: ed.Name, Namespace: ed.Namespace}, serviceRoute)
	if !ed.Route.TLS.Enabled || (ed.Route.TLS.Enabled && err != nil && k8s_errors.IsNotFound(err)) {
		ctrlResult, err := endptRoute.CreateOrPatch(ctx, helper)
		if (err != nil) || (ctrlResult != ctrl.Result{}) {
			return ctrlResult, err
		}

		ed.Hostname = ptr.To(endptRoute.GetHostname())
	} else if err != nil {
		return ctrl.Result{}, err
	} else {
		ed.Hostname = &serviceRoute.Spec.Host
	}

	// if TLS is enabled for the route
	if ed.Route.TLS.Enabled {
		var ctrlResult reconcile.Result

		certSecret := &k8s_corev1.Secret{}

		// if a custom cert secret was provided, check if it exist
		// and has the required cert, key and cacert
		// Right now there is no check if certificate is valid for
		// the hostname of the route. If the referenced secret is
		// there and has the required files it is just being used.
		if ed.Route.TLS.SecretName != nil {
			certSecret, _, err = secret.GetSecret(ctx, helper, *ed.Route.TLS.SecretName, ed.Namespace)
			if err != nil {
				if k8s_errors.IsNotFound(err) {
					return ctrl.Result{}, fmt.Errorf("certificate secret %s not found: %w", *ed.Route.TLS.SecretName, err)
				}

				return ctrl.Result{}, err
			}

			// check if secret has the expected entries tls.crt, tls.key and ca.crt
			if certSecret != nil {
				for _, key := range []string{"tls.crt", "tls.key", "ca.crt"} {
					if _, exist := certSecret.Data[key]; !exist {
						return ctrl.Result{}, fmt.Errorf("certificate secret %s does not provide %s", *ed.Route.TLS.SecretName, key)
					}
				}
			}
		}

		// create default TLS route override
		tlsConfig := &routev1.TLSConfig{
			Termination:                   routev1.TLSTerminationEdge,
			Certificate:                   string(certSecret.Data[tls.CertKey]),
			Key:                           string(certSecret.Data[tls.PrivateKey]),
			CACertificate:                 string(certSecret.Data[tls.CAKey]),
			InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyRedirect,
		}

		// for internal TLS (TLSE) use routev1.TLSTerminationReencrypt
		if ed.Service.TLS.Enabled && (ed.Service.TLS.SecretName != nil || hasCertInOverrideSpec(ed.Route.OverrideSpec)) {
			// get the TLSInternalCABundleFile to add it to the route
			// to be able to validate public/internal service endpoints
			tlsConfig.DestinationCACertificate, ctrlResult, err = secret.GetDataFromSecret(
				ctx, helper, ed.Service.TLS.CaBundleSecretName, 5, tls.InternalCABundleKey,
			)
			if (err != nil) || (ctrlResult != ctrl.Result{}) {
				return ctrlResult, err
			}

			tlsConfig.Termination = routev1.TLSTerminationReencrypt
		}

		endptRoute, err = route.NewRoute(
			endptRoute.GetRoute(),
			time.Duration(5)*time.Second,
			[]route.OverrideSpec{
				{
					Spec: &route.Spec{
						TLS: tlsConfig,
					},
				},
				ed.Route.OverrideSpec,
			},
		)
		if err != nil {
			return ctrl.Result{}, err
		}

		ctrlResult, err = endptRoute.CreateOrPatch(ctx, helper)
		if (err != nil) || (ctrlResult != ctrl.Result{}) {
			return ctrlResult, err
		}
		ed.Proto = service.ProtocolHTTPS
	} else {
		ed.Proto = service.ProtocolHTTP
	}

	ed.EndpointURL = ed.Proto.String() + "://" + *ed.Hostname

	return ctrl.Result{}, nil
}

func hasCertInOverrideSpec(overrideSpec route.OverrideSpec) bool {
	if overrideSpec.Spec == nil {
		return false
	}
	if overrideSpec.Spec.TLS == nil {
		return false
	}
	return overrideSpec.Spec.TLS.CACertificate != "" &&
		overrideSpec.Spec.TLS.Certificate != "" &&
		overrideSpec.Spec.TLS.Key != ""
}

// GetRoutesListWithLabel - Get all routes in namespace of the obj matching label selector
func GetRoutesListWithLabel(
	ctx context.Context,
	h *helper.Helper,
	namespace string,
	labelSelectorMap map[string]string,
) (*routev1.RouteList, error) {
	routeList := &routev1.RouteList{}
	listOpts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels(labelSelectorMap),
	}

	if err := h.GetClient().List(ctx, routeList, listOpts...); err != nil {
		err = fmt.Errorf("Error listing routes for %s: %w", labelSelectorMap, err)
		return nil, err
	}

	return routeList, nil
}

// end of helper types to expose services

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
//+kubebuilder:rbac:groups="",resources=pods,verbs=create;delete;get;list;patch;update;watch
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete;
//+kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch;delete;
//+kubebuilder:rbac:groups=route.openshift.io,resources=routes/custom-host,verbs=create;update;patch

// service account, role, rolebinding
//+kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch
//+kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles,verbs=get;list;watch;create;update;patch
//+kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;list;watch;create;update;patch
//+kubebuilder:rbac:groups="security.openshift.io",resourceNames=anyuid,resources=securitycontextconstraints,verbs=use

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

	// initialize the rbac
	err = r.ensureRbac(ctx, helper, instance)
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
	// Top level secret
	hash, _, inputSecret, err := ensureSecret(
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

	// TransportURL Secret
	hashTransporturl, _, transporturlSecret, err := ensureSecret(
		ctx,
		types.NamespacedName{Namespace: instance.Namespace, Name: transportURL.Status.SecretName},
		[]string{
			TransportURLSelector,
		},
		helper.GetClient(),
		&instance.Status.Conditions,
		r.RequeueTimeout,
	)
	if err != nil || hashTransporturl == "" {
		// Empty hash means that there is some problem retrieving the key from the secret
		return ctrl.Result{}, errors.New("error retrieving required data from transporturl secret")
	}

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

	// Add finalizer to prometheus config secret to prevent it from being deleted now that we're using it
	if controllerutil.AddFinalizer(&prometheusSecret, helper.GetFinalizer()) {
		err := helper.GetClient().Update(ctx, &prometheusSecret)
		if err != nil {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.RequestedReason,
				condition.SeverityWarning,
				watcherv1beta1.WatcherPrometheusSecretErrorMessage))
			return ctrl.Result{}, err
		}
	}

	// End of Prometheus config secret

	subLevelSecretName, err := r.createSubLevelSecret(ctx, helper, instance, transporturlSecret, inputSecret, db)
	if err != nil {
		return ctrl.Result{}, nil
	}

	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)
	// End of Input Ready check

	// Create Keystone Service creation. The endpoint will be created by WatcherAPI
	_, err = r.ensureKeystoneSvc(ctx, helper, instance, serviceLabels)

	if err != nil {

		return ctrl.Result{}, err
	}

	// End of Keystone service creation

	// Generate config for dbsync
	configVars := make(map[string]env.Setter)

	err = r.generateServiceConfigDBJobs(ctx, instance, db, &transporturlSecret, helper, &configVars)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	instance.Status.Conditions.MarkTrue(condition.ServiceConfigReadyCondition, condition.ServiceConfigReadyMessage)
	// End of config generation for dbsync

	// Create dbsync job
	ctrlResult, err := r.ensureDBSync(ctx, helper, instance, serviceLabels)
	if err != nil {
		return ctrl.Result{}, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// End of creation of job for dbsync

	// Create DBPurge CronJob
	err = r.ensureDBPurgeCronJob(ctx, helper, instance, serviceLabels)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Create Watcher API
	_, _, err = r.ensureAPI(ctx, instance, subLevelSecretName)

	if err != nil {
		return ctrl.Result{}, err
	}

	ctrlResult, err = r.exposeEndpoints(
		ctx,
		helper,
		instance,
	)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ExposeServiceReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ExposeServiceReadyErrorMessage,
			err.Error(),
		))
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ExposeServiceReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.ExposeServiceReadyRunningMessage,
		))
		return ctrlResult, nil
	}

	instance.Status.Conditions.MarkTrue(condition.ExposeServiceReadyCondition, condition.ExposeServiceReadyMessage)

	// End of Watcher API creation

	// Create Watcher DecisionEngine
	_, _, err = r.ensureDecisionEngine(ctx, instance, subLevelSecretName)

	if err != nil {
		return ctrl.Result{}, err
	}

	// End of Watcher DecisionEngine creation

	// Deploy Watcher Applier
	_, _, err = r.ensureApplier(ctx, instance, subLevelSecretName)

	if err != nil {
		return ctrl.Result{}, err
	}
	// End of Watcher Applier deploy

	//
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

	// initialize .Status.Hash
	if instance.Status.Hash == nil {
		instance.Status.Hash = make(map[string]string)
	}

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
		condition.UnknownCondition(
			condition.ServiceAccountReadyCondition,
			condition.InitReason,
			condition.ServiceAccountReadyInitMessage),
		condition.UnknownCondition(
			condition.RoleReadyCondition,
			condition.InitReason,
			condition.RoleReadyInitMessage),
		condition.UnknownCondition(
			condition.RoleBindingReadyCondition,
			condition.InitReason,
			condition.RoleBindingReadyInitMessage),
		condition.UnknownCondition(
			condition.ServiceConfigReadyCondition,
			condition.InitReason,
			condition.ServiceConfigReadyInitMessage),
		condition.UnknownCondition(
			condition.DBSyncReadyCondition,
			condition.InitReason,
			condition.DBSyncReadyInitMessage),
		condition.UnknownCondition(
			watcherv1beta1.WatcherAPIReadyCondition,
			condition.InitReason,
			watcherv1beta1.WatcherAPIReadyInitMessage),
		condition.UnknownCondition(
			watcherv1beta1.WatcherApplierReadyCondition,
			condition.InitReason,
			watcherv1beta1.WatcherApplierReadyInitMessage),
		condition.UnknownCondition(
			watcherv1beta1.WatcherDecisionEngineReadyCondition,
			condition.InitReason,
			watcherv1beta1.WatcherDecisionEngineReadyInitMessage),
		condition.UnknownCondition(
			condition.CronJobReadyCondition,
			condition.InitReason,
			condition.CronJobReadyInitMessage),
		condition.UnknownCondition(
			condition.ExposeServiceReadyCondition,
			condition.InitReason,
			condition.ExposeServiceReadyInitMessage),
	)

	instance.Status.Conditions.Init(&cl)

	return nil
}

// Create ServiceAccount, Role and RoleBinding
func (r *WatcherReconciler) ensureRbac(
	ctx context.Context,
	h *helper.Helper,
	instance *watcherv1beta1.Watcher) error {
	// Service account, role, binding
	rbacRules := []rbacv1.PolicyRule{
		{
			APIGroups:     []string{"security.openshift.io"},
			ResourceNames: []string{"anyuid"},
			Resources:     []string{"securitycontextconstraints"},
			Verbs:         []string{"use"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"create", "get", "list", "watch", "update", "patch", "delete"},
		},
	}

	rbacResult, err := common_rbac.ReconcileRbac(ctx, h, instance, rbacRules)
	if err != nil {
		return err
	} else if (rbacResult != ctrl.Result{}) {
		return nil
	}

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
		*instance.Spec.DatabaseInstance, // mariadb/galera service to target
		watcher.DatabaseName,            // name used in CREATE DATABASE in mariadb
		watcher.DatabaseCRName,          // CR name for MariaDBDatabase
		instance.Spec.DatabaseAccount,   // CR name for MariaDBAccount
		instance.Namespace,              // namespace
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
		transportURL.Spec.RabbitmqClusterName = *instance.Spec.RabbitMqClusterName

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

func (r *WatcherReconciler) generateServiceConfigDBJobs(
	ctx context.Context,
	instance *watcherv1beta1.Watcher,
	db *mariadbv1.Database,
	transporturlSecret *corev1.Secret,
	helper *helper.Helper,
	envVars *map[string]env.Setter,
) error {
	Log := r.GetLogger(ctx)
	Log.Info("generateServiceConfigs - reconciling config for Watcher CR")

	var tlsCfg *tls.Service
	if instance.Spec.APIServiceTemplate.TLS.Ca.CaBundleSecretName != "" {
		tlsCfg = &tls.Service{}
	}
	// customData hold any customization for the service.
	customData := map[string]string{
		watcher.GlobalCustomConfigFileName: instance.Spec.CustomServiceConfig,
		"my.cnf":                           db.GetDatabaseClientConfig(tlsCfg), //(mschuppert) for now just get the default my.cnf
	}

	labels := labels.GetLabels(instance, labels.GetGroupLabel(watcher.ServiceName), map[string]string{})
	databaseAccount := db.GetAccount()
	databaseSecret := db.GetSecret()
	templateParameters := map[string]interface{}{
		"DatabaseConnection": fmt.Sprintf("mysql+pymysql://%s:%s@%s/%s?read_default_file=/etc/my.cnf",
			databaseAccount.Spec.UserName,
			string(databaseSecret.Data[mariadbv1.DatabasePasswordSelector]),
			db.GetDatabaseHostname(),
			watcher.DatabaseName,
		),
		"TransportURL":  string(transporturlSecret.Data[TransportURLSelector]),
		"LogFile":       fmt.Sprintf("%s%s.log", watcher.WatcherLogPath, instance.Name),
		"APIPublicPort": fmt.Sprintf("%d", watcher.WatcherPublicPort),
	}

	return GenerateConfigsGeneric(ctx, helper, instance, envVars, templateParameters, customData, labels, true)
}

func (r *WatcherReconciler) ensureDBSync(
	ctx context.Context,
	h *helper.Helper,
	instance *watcherv1beta1.Watcher,
	serviceLabels map[string]string,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info(fmt.Sprintf("Reconciling the Keystone Service for '%s'", instance.Name))

	// So far we are not using Service Annotations
	dbSyncHash := instance.Status.Hash[watcherv1beta1.DbSyncHash]
	jobDef := watcher.DbSyncJob(instance, serviceLabels, nil)

	dbSyncjob := job.NewJob(
		jobDef,
		watcherv1beta1.DbSyncHash,
		instance.Spec.PreserveJobs,
		time.Duration(5)*time.Second,
		dbSyncHash,
	)

	ctrlResult, err := dbSyncjob.DoJob(
		ctx,
		h,
	)

	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBSyncReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DBSyncReadyRunningMessage))
		return ctrlResult, nil
	}
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBSyncReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DBSyncReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	if dbSyncjob.HasChanged() {
		instance.Status.Hash[watcherv1beta1.DbSyncHash] = dbSyncjob.GetHash()
		Log.Info(fmt.Sprintf("Service '%s' - Job %s hash added - %s", instance.Name, jobDef.Name, instance.Status.Hash[watcherv1beta1.DbSyncHash]))
	}
	instance.Status.Conditions.MarkTrue(condition.DBSyncReadyCondition, condition.DBSyncReadyMessage)

	return ctrlResult, nil
}

func (r *WatcherReconciler) createSubLevelSecret(
	ctx context.Context,
	helper *helper.Helper,
	instance *watcherv1beta1.Watcher,
	transportURLSecret corev1.Secret,
	inputSecret corev1.Secret,
	db *mariadbv1.Database,
) (string, error) {
	Log := r.GetLogger(ctx)
	Log.Info(fmt.Sprintf("Creating SubCr Level Secret for '%s'", instance.Name))
	databaseAccount := db.GetAccount()
	databaseSecret := db.GetSecret()
	data := map[string]string{
		instance.Spec.PasswordSelectors.Service: string(inputSecret.Data[instance.Spec.PasswordSelectors.Service]),
		TransportURLSelector:                    string(transportURLSecret.Data[TransportURLSelector]),
		DatabaseAccount:                         databaseAccount.Name,
		DatabaseUsername:                        databaseAccount.Spec.UserName,
		DatabasePassword:                        string(databaseSecret.Data[mariadbv1.DatabasePasswordSelector]),
		DatabaseHostname:                        db.GetDatabaseHostname(),
		watcher.GlobalCustomConfigFileName:      instance.Spec.CustomServiceConfig,
	}
	secretName := instance.Name

	labels := labels.GetLabels(instance, labels.GetGroupLabel(watcher.ServiceName), map[string]string{})

	template := util.Template{
		Name:         secretName,
		Namespace:    instance.Namespace,
		Type:         util.TemplateTypeNone,
		InstanceType: instance.GetObjectKind().GroupVersionKind().Kind,
		Labels:       labels,
		CustomData:   data,
	}

	err := secret.EnsureSecrets(ctx, helper, instance, []util.Template{template}, nil)

	return secretName, err
}

func (r *WatcherReconciler) ensureAPI(
	ctx context.Context,
	instance *watcherv1beta1.Watcher,
	secretName string,
) (*watcherv1beta1.WatcherAPI, controllerutil.OperationResult, error) {
	Log := r.GetLogger(ctx)
	Log.Info(fmt.Sprintf("Creating WatcherAPI '%s'", instance.Name))

	// If no TopologyRef is included in the APIServiceTemplate, override it with the
	// top level one.
	if instance.Spec.APIServiceTemplate.TopologyRef == nil {
		instance.Spec.APIServiceTemplate.TopologyRef = instance.Spec.TopologyRef
	}

	watcherAPISpec := watcherv1beta1.WatcherAPISpec{
		Secret: secretName,
		WatcherCommon: watcherv1beta1.WatcherCommon{
			ServiceUser:         instance.Spec.ServiceUser,
			PasswordSelectors:   instance.Spec.PasswordSelectors,
			MemcachedInstance:   instance.Spec.MemcachedInstance,
			NodeSelector:        instance.Spec.APIServiceTemplate.NodeSelector,
			PreserveJobs:        instance.Spec.PreserveJobs,
			CustomServiceConfig: instance.Spec.APIServiceTemplate.CustomServiceConfig,
			TopologyRef:         instance.Spec.APIServiceTemplate.TopologyRef,
		},
		WatcherSubCrsCommon: watcherv1beta1.WatcherSubCrsCommon{
			ContainerImage: instance.Spec.APIContainerImageURL,
			Resources:      instance.Spec.APIServiceTemplate.Resources,
			ServiceAccount: "watcher-" + instance.Name,
		},
		Replicas: instance.Spec.APIServiceTemplate.Replicas,
		Override: instance.Spec.APIServiceTemplate.Override,
		TLS:      instance.Spec.APIServiceTemplate.TLS,
	}

	// If NodeSelector is not specified in Watcher APIServiceTemplate, the current
	// API instance inherits the value from the top-level Watcher CR.
	if watcherAPISpec.NodeSelector == nil {
		watcherAPISpec.NodeSelector = instance.Spec.NodeSelector
	}

	// We need to have the PrometheusSecret in watcherapi
	watcherAPISpec.PrometheusSecret = instance.Spec.PrometheusSecret

	apiDeployment := &watcherv1beta1.WatcherAPI{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-api", instance.Name),
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrPatch(ctx, r.Client, apiDeployment, func() error {
		apiDeployment.Spec = watcherAPISpec
		err := controllerutil.SetControllerReference(instance, apiDeployment, r.Scheme)
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			watcherv1beta1.WatcherAPIReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			watcherv1beta1.WatcherAPIReadyErrorMessage,
			err.Error()))
		return nil, op, err
	}
	if op != controllerutil.OperationResultNone {
		Log.Info(fmt.Sprintf("WatcherAPI %s , WatcherAPI.Name %s.", string(op), apiDeployment.Name))
	}

	if apiDeployment.Generation == apiDeployment.Status.ObservedGeneration {
		c := apiDeployment.Status.Conditions.Mirror(watcherv1beta1.WatcherAPIReadyCondition)
		// NOTE(gibi): it can be nil if the WatcherAPI CR is created but no
		// reconciliation is run on it to initialize the ReadyCondition yet.
		if c != nil {
			instance.Status.Conditions.Set(c)
		}
		instance.Status.APIServiceReadyCount = apiDeployment.Status.ReadyCount
	}

	return apiDeployment, op, nil

}

func (r *WatcherReconciler) ensureApplier(
	ctx context.Context,
	instance *watcherv1beta1.Watcher,
	secretName string,
) (*watcherv1beta1.WatcherApplier, controllerutil.OperationResult, error) {
	Log := r.GetLogger(ctx)
	Log.Info(fmt.Sprintf("Creating WatcherApplier '%s'", instance.Name))

	// If no TopologyRef is included in the ApplierServiceTemplate, override it with the
	// top level one.
	if instance.Spec.ApplierServiceTemplate.TopologyRef == nil {
		instance.Spec.ApplierServiceTemplate.TopologyRef = instance.Spec.TopologyRef
	}

	watcherApplierSpec := watcherv1beta1.WatcherApplierSpec{
		Secret: secretName,
		WatcherCommon: watcherv1beta1.WatcherCommon{
			ServiceUser:         instance.Spec.ServiceUser,
			PasswordSelectors:   instance.Spec.PasswordSelectors,
			MemcachedInstance:   instance.Spec.MemcachedInstance,
			NodeSelector:        instance.Spec.ApplierServiceTemplate.NodeSelector,
			PreserveJobs:        instance.Spec.PreserveJobs,
			CustomServiceConfig: instance.Spec.ApplierServiceTemplate.CustomServiceConfig,
			TopologyRef:         instance.Spec.ApplierServiceTemplate.TopologyRef,
		},
		WatcherSubCrsCommon: watcherv1beta1.WatcherSubCrsCommon{
			ContainerImage: instance.Spec.ApplierContainerImageURL,
			Resources:      instance.Spec.ApplierServiceTemplate.Resources,
			ServiceAccount: "watcher-" + instance.Name,
		},
		Replicas: instance.Spec.ApplierServiceTemplate.Replicas,
		TLS:      instance.Spec.APIServiceTemplate.TLS.Ca,
	}

	// If NodeSelector is not specified in Watcher ApplierServiceTemplate,
	// the instance inherits the value from the top-level Watcher CR.
	if watcherApplierSpec.NodeSelector == nil {
		watcherApplierSpec.NodeSelector = instance.Spec.NodeSelector
	}

	applierDeployment := &watcherv1beta1.WatcherApplier{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-applier", instance.Name),
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrPatch(ctx, r.Client, applierDeployment, func() error {
		applierDeployment.Spec = watcherApplierSpec
		err := controllerutil.SetControllerReference(instance, applierDeployment, r.Scheme)
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			watcherv1beta1.WatcherApplierReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			watcherv1beta1.WatcherApplierReadyErrorMessage,
			err.Error()))
		return nil, op, err
	}
	if op != controllerutil.OperationResultNone {
		Log.Info(fmt.Sprintf("WatcherApplier %s , WatcherApplier.Name %s.", string(op), applierDeployment.Name))
	}

	if applierDeployment.Generation == applierDeployment.Status.ObservedGeneration {
		c := applierDeployment.Status.Conditions.Mirror(watcherv1beta1.WatcherApplierReadyCondition)
		// NOTE(gibi): it can be nil if the WatcherApplier CR is created but no
		// reconciliation is run on it to initialize the ReadyCondition yet.
		if c != nil {
			instance.Status.Conditions.Set(c)
		}
		instance.Status.ApplierServiceReadyCount = applierDeployment.Status.ReadyCount
	}

	return applierDeployment, op, nil

}

func (r *WatcherReconciler) ensureDecisionEngine(
	ctx context.Context,
	instance *watcherv1beta1.Watcher,
	secretName string,
) (*watcherv1beta1.WatcherDecisionEngine, controllerutil.OperationResult, error) {
	Log := r.GetLogger(ctx)
	Log.Info(fmt.Sprintf("Creating WatcherDecisionEngine '%s'", instance.Name))

	// If no TopologyRef is included in the DecisionEngineServiceTemplate, override it with the
	// top level one.
	if instance.Spec.DecisionEngineServiceTemplate.TopologyRef == nil {
		instance.Spec.DecisionEngineServiceTemplate.TopologyRef = instance.Spec.TopologyRef
	}

	watcherDecisionEngineSpec := watcherv1beta1.WatcherDecisionEngineSpec{
		Secret: secretName,
		WatcherCommon: watcherv1beta1.WatcherCommon{
			ServiceUser:         instance.Spec.ServiceUser,
			PasswordSelectors:   instance.Spec.PasswordSelectors,
			MemcachedInstance:   instance.Spec.MemcachedInstance,
			NodeSelector:        instance.Spec.DecisionEngineServiceTemplate.NodeSelector,
			PreserveJobs:        instance.Spec.PreserveJobs,
			CustomServiceConfig: instance.Spec.DecisionEngineServiceTemplate.CustomServiceConfig,
			TopologyRef:         instance.Spec.DecisionEngineServiceTemplate.TopologyRef,
		},
		WatcherSubCrsCommon: watcherv1beta1.WatcherSubCrsCommon{
			ContainerImage: instance.Spec.DecisionEngineContainerImageURL,
			Resources:      instance.Spec.DecisionEngineServiceTemplate.Resources,
			ServiceAccount: "watcher-" + instance.Name,
		},
		Replicas: instance.Spec.DecisionEngineServiceTemplate.Replicas,
		TLS:      instance.Spec.APIServiceTemplate.TLS.Ca,
	}

	// If NodeSelector is not specified in Watcher DecisionEngineServiceTemplate, the current
	// DecisionEngine instance inherits the value from the top-level Watcher CR.
	if watcherDecisionEngineSpec.NodeSelector == nil {
		watcherDecisionEngineSpec.NodeSelector = instance.Spec.NodeSelector
	}

	// We need to have the PrometheusSecret in watcherdecisionengine
	watcherDecisionEngineSpec.PrometheusSecret = instance.Spec.PrometheusSecret

	decisionengineDeployment := &watcherv1beta1.WatcherDecisionEngine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-decision-engine", instance.Name),
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrPatch(ctx, r.Client, decisionengineDeployment, func() error {
		decisionengineDeployment.Spec = watcherDecisionEngineSpec
		err := controllerutil.SetControllerReference(instance, decisionengineDeployment, r.Scheme)
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			watcherv1beta1.WatcherDecisionEngineReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			watcherv1beta1.WatcherDecisionEngineReadyErrorMessage,
			err.Error()))
		return nil, op, err
	}
	if op != controllerutil.OperationResultNone {
		Log.Info(fmt.Sprintf("WatcherDecisionEngine %s , WatcherDecisionEngine.Name %s.", string(op), decisionengineDeployment.Name))
	}

	if decisionengineDeployment.Generation == decisionengineDeployment.Status.ObservedGeneration {
		c := decisionengineDeployment.Status.Conditions.Mirror(watcherv1beta1.WatcherDecisionEngineReadyCondition)
		// NOTE(gibi): it can be nil if the WatcherDecisionEngine CR is created but no
		// reconciliation is run on it to initialize the ReadyCondition yet.
		if c != nil {
			instance.Status.Conditions.Set(c)
		}
		instance.Status.DecisionEngineServiceReadyCount = decisionengineDeployment.Status.ReadyCount
	}

	return decisionengineDeployment, op, nil
}

func (r *WatcherReconciler) ensureDBPurgeCronJob(
	ctx context.Context,
	h *helper.Helper,
	instance *watcherv1beta1.Watcher,
	serviceLabels map[string]string,
) error {

	cronDef := watcher.DBPurgeCronJob(instance, serviceLabels, nil)
	cronjob := cronjob.NewCronJob(cronDef, r.RequeueTimeout)

	_, err := cronjob.CreateOrPatch(ctx, h)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.CronJobReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.CronJobReadyErrorMessage,
			err.Error()))
		return err
	}

	instance.Status.Conditions.MarkTrue(
		condition.CronJobReadyCondition, condition.CronJobReadyMessage)
	return nil
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

	// Remove the finalizer from our Prometheus Secret
	prometheusSecret := &corev1.Secret{}
	reader := helper.GetClient()
	err = reader.Get(ctx,
		types.NamespacedName{Namespace: instance.Namespace, Name: instance.Spec.PrometheusSecret},
		prometheusSecret)

	if err == nil {
		if controllerutil.RemoveFinalizer(prometheusSecret, helper.GetFinalizer()) {
			err = helper.GetClient().Update(ctx, prometheusSecret)
			if err != nil && !k8s_errors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			util.LogForObject(helper, "Removed finalizer from prometheus config secret", instance)
		}
	}
	//

	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
	Log.Info(fmt.Sprintf("Reconciled Service '%s' delete successfully", instance.Name))
	return ctrl.Result{}, nil
}

func (r *WatcherReconciler) exposeEndpoints(
	ctx context.Context,
	helper *helper.Helper,
	instance *watcherv1beta1.Watcher,

) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info(fmt.Sprintf("Exposing WatcherAPI services '%s'", instance.Name))

	svcs, err := service.GetServicesListWithLabel(
		ctx,
		helper,
		instance.Namespace,
		getAPIServiceLabels(),
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	// validate that ingress TLS is enabled when PodLevel TLS is enabled
	hasIngressTLS := instance.Spec.APIOverride.TLS != nil && instance.Spec.APIOverride.TLS.SecretName != ""
	hasPublicServiceSecretName := instance.Spec.APIServiceTemplate.TLS.API.Public.SecretName != nil &&
		*instance.Spec.APIServiceTemplate.TLS.API.Public.SecretName != ""
	if hasPublicServiceSecretName && !hasIngressTLS {
		err = fmt.Errorf("TLS at ingress level is not configured, but at PodLevel is enabled, please set a secret to enable TLS on ingress")
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ExposeServiceReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ExposeServiceReadyErrorMessage,
			err.Error(),
		))
		return ctrl.Result{}, err
	}

	for _, svc := range svcs.Items {
		ed := EndpointDetail{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			Type:      service.Endpoint(svc.Annotations[service.AnnotationEndpointKey]),
			Service: ServiceDetails{
				Spec: &svc,
			},
		}

		ed.Service.OverrideSpec = instance.Spec.APIServiceTemplate.Override.Service[ed.Type]

		if ed.Type == service.EndpointPublic {

			// if the user has passed a secretName, we want to use TLS on the
			// pod level
			ed.Service.TLS.Enabled = hasPublicServiceSecretName

			// If the service has the create ingress annotation and its
			// a default ClusterIP service -> create route
			ed.Route.Create = svc.ObjectMeta.Annotations[service.AnnotationIngressCreateKey] == "true" &&
				svc.Spec.Type == k8s_corev1.ServiceTypeClusterIP

			if instance.Spec.APIOverride.Route != nil {
				ed.Route.OverrideSpec = *instance.Spec.APIOverride.Route
			}

			if hasIngressTLS {
				// TLS for route enabled if public endpoint TLS is true
				ed.Route.TLS.Enabled = true
				ed.Route.TLS.CertName = fmt.Sprintf("%s-route", ed.Name)

				ed.Route.TLS.SecretName = ptr.To(instance.Spec.APIOverride.TLS.SecretName)
				validateSecret := &tls.GenericService{SecretName: ed.Route.TLS.SecretName}
				_, err := validateSecret.ValidateCertSecret(ctx, helper, instance.GetNamespace())
				if err != nil {
					if k8s_errors.IsNotFound(err) {
						return ctrl.Result{RequeueAfter: time.Duration(10) * time.Second}, nil
					}
					return ctrl.Result{}, err
				}
			}

			if ed.Service.TLS.Enabled {
				ed.Service.TLS.CaBundleSecretName = tls.CABundleSecret
				ed.Service.TLS.SecretName = instance.Spec.APIServiceTemplate.TLS.API.Public.SecretName
				_, err := ed.Service.TLS.GenericService.ValidateCertSecret(ctx, helper, instance.Namespace)
				if err != nil {
					if k8s_errors.IsNotFound(err) {
						return ctrl.Result{RequeueAfter: time.Duration(10) * time.Second}, nil
					}
					return ctrl.Result{}, err
				}
			}

			ctrlResult, err := ed.ensureRoute(ctx, helper, instance, &svc)
			if err != nil || (ctrlResult != ctrl.Result{}) {
				return ctrlResult, err
			}
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WatcherReconciler) SetupWithManager(mgr ctrl.Manager) error {

	// index passwordSecretField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &watcherv1beta1.Watcher{}, passwordSecretField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*watcherv1beta1.Watcher)
		if cr.Spec.Secret == "" {
			return nil
		}
		return []string{cr.Spec.Secret}
	}); err != nil {
		return err
	}

	// index prometheusSecretField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &watcherv1beta1.Watcher{}, prometheusSecretField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*watcherv1beta1.Watcher)
		if cr.Spec.PrometheusSecret == "" {
			return nil
		}
		return []string{cr.Spec.PrometheusSecret}
	}); err != nil {
		return err
	}

	// index tlsRouteSecretField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &watcherv1beta1.Watcher{}, tlsRouteSecretField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*watcherv1beta1.Watcher)
		if cr.Spec.APIOverride.TLS == nil || cr.Spec.APIOverride.TLS.SecretName == "" {
			return nil
		}
		return []string{cr.Spec.APIOverride.TLS.SecretName}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&watcherv1beta1.Watcher{}).
		Owns(&watcherv1beta1.WatcherAPI{}).
		Owns(&watcherv1beta1.WatcherDecisionEngine{}).
		Owns(&watcherv1beta1.WatcherApplier{}).
		Owns(&mariadbv1.MariaDBDatabase{}).
		Owns(&mariadbv1.MariaDBAccount{}).
		Owns(&rabbitmqv1.TransportURL{}).
		Owns(&keystonev1.KeystoneService{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&batchv1.Job{}).
		Owns(&batchv1.CronJob{}).
		Owns(&corev1.Secret{}).
		Owns(&routev1.Route{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForSrc),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}

func (r *WatcherReconciler) findObjectsForSrc(ctx context.Context, src client.Object) []reconcile.Request {
	requests := []reconcile.Request{}

	l := log.FromContext(ctx).WithName("Controllers").WithName("Watcher")

	for _, field := range watcherWatchFields {
		crList := &watcherv1beta1.WatcherList{}
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
