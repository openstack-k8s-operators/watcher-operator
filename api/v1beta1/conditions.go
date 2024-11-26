package v1beta1

import "github.com/openstack-k8s-operators/lib-common/modules/common/condition"

const (
	// WatcherRabbitMQTransportURLReadyCondition -
	WatcherRabbitMQTransportURLReadyCondition condition.Type = "WatcherRabbitMQTransportURLReady"
)

const (
	// WatcherRabbitMQTransportURLReadyRunningMessage -
	WatcherRabbitMQTransportURLReadyRunningMessage = "WatcherRabbitMQTransportURL creation in progress"
	// WatcherRabbitMQTransportURLReadyMessage -
	WatcherRabbitMQTransportURLReadyMessage = "WatcherRabbitMQTransportURL successfully created"
	// WatcherRabbitMQTransportURLReadyErrorMessage -
	WatcherRabbitMQTransportURLReadyErrorMessage = "WatcherRabbitMQTransportURL error occured %s"
)
