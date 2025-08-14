package v1beta1

import "github.com/openstack-k8s-operators/lib-common/modules/common/condition"

const (
	// WatcherRabbitMQTransportURLReadyCondition -
	WatcherRabbitMQTransportURLReadyCondition condition.Type = "WatcherRabbitMQTransportURLReady"
	// WatcherNotificationsBusTransportURLReadyCondition -
	WatcherNotificationTransportURLReadyCondition condition.Type = "WatcherNotificationTransportURLReady"
	// WatcherAPIReadyCondition -
	WatcherAPIReadyCondition condition.Type = "WatcherAPIReady"
	// WatcherApplierReadyCondition -
	WatcherApplierReadyCondition condition.Type = "WatcherApplierReady"
	// WatcherDecisionEngineReadyCondition -
	WatcherDecisionEngineReadyCondition condition.Type = "WatcherDecisionEngineReady"
)

const (
	// WatcherRabbitMQTransportURLReadyRunningMessage -
	WatcherRabbitMQTransportURLReadyRunningMessage = "WatcherRabbitMQTransportURL creation in progress"
	// WatcherRabbitMQTransportURLReadyMessage -
	WatcherRabbitMQTransportURLReadyMessage = "WatcherRabbitMQTransportURL successfully created"
	// WatcherRabbitMQTransportURLReadyErrorMessage -
	WatcherRabbitMQTransportURLReadyErrorMessage = "WatcherRabbitMQTransportURL error occured %s"
	// WatcherNotificationTransportURLReadyRunningMessage -
	WatcherNotificationTransportURLReadyRunningMessage = "WatcherNotificationTransportURL creation in progress"
	// WatcherNotificationTransportURLReadyMessage -
	WatcherNotificationTransportURLReadyMessage = "WatcherNotificationTransportURL successfully created"
	// WatcherNotificationTransportURLReadyErrorMessage -
	WatcherNotificationTransportURLReadyErrorMessage = "WatcherNotificationTransportURL error occured %s"
	// WatcherAPIReadyInitMessage -
	WatcherAPIReadyInitMessage = "WatcherAPI creation not started"
	// WatcherAPIReadyRunningMessage -
	WatcherAPIReadyRunningMessage = "WatcherAPI creation in progress"
	// WatcherAPIReadyMessage -
	WatcherAPIReadyMessage = "WatcherAPI successfully created"
	// WatcherAPIReadyErrorMessage -
	WatcherAPIReadyErrorMessage = "WatcherAPI error occured %s"
	// WatcherPrometheusSecretErrorMessage -
	WatcherPrometheusSecretErrorMessage = "Error with prometheus config secret"
	// WatcherApplierReadyInitMessage -
	WatcherApplierReadyInitMessage = "WatcherApplier creation not started"
	// WatcherApplierReadyRunningMessage -
	WatcherApplierReadyRunningMessage = "WatcherApplier creation in progress"
	// WatcherApplierReadyMessage -
	WatcherApplierReadyMessage = "WatcherApplier successfully created"
	// WatcherApplierReadyErrorMessage -
	WatcherApplierReadyErrorMessage = "WatcherApplier error occured %s"
	// WatcherDecisionEngineReadyInitMessage -
	WatcherDecisionEngineReadyInitMessage = "WatcherDecisionEngine creation not started"
	// WatcherDecisionEngineReadyRunningMessage -
	WatcherDecisionEngineReadyRunningMessage = "WatcherDecisionEngine creation in progress"
	// WatcherDecisionEngineReadyMessage -
	WatcherDecisionEngineReadyMessage = "WatcherDecisionEngine successfully created"
	// WatcherDecisionEngineReadyErrorMessage -
	WatcherDecisionEngineReadyErrorMessage = "WatcherDecisionEngine error occured %s"
)
