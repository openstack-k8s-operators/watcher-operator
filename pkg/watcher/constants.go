package watcher

const (
	// ServiceName - Name to identify watcher service
	ServiceName = "watcher"

	// ServiceType - Type of Watcher keystone service
	ServiceType = "infra-optim"

	// DatabaseName - Name of the database used in CREATE DATABASE statement
	DatabaseName = "watcher"

	// DatabaseUsernamePrefix - used by EnsureMariaDBAccount when a new username
	// is created.
	DatabaseUsernamePrefix = "watcher"

	// DatabaseCRName - name of the CR used to create the Watcher database
	DatabaseCRName = "watcher"

	// DefaultsConfigFileName - File name with default configuration
	DefaultsConfigFileName = "00-default.conf"

	// GlobalCustomConfigFileName - File name with custom configuration defined in Watcher
	GlobalCustomConfigFileName = "01-global-custom.conf"

	// ServiceCustomConfigFileName - File name with custom configuration defined in SubCRs
	ServiceCustomConfigFileName = "02-service-custom.conf"

	// LogVolume is the default logVolume name used to mount logs
	LogVolume = "logs"

	// WatcherPublicPort - public port of watcher containers
	WatcherPublicPort int32 = 9322

	// WatcherPublicPort - internal port of watcher containers
	WatcherInternalPort int32 = 9322

	// WatcherLogPath is the path used by WatcherAPI to stream/store its logs
	WatcherLogPath = "/var/log/watcher/"

	// ConfigVolume is the default volume name used to mount service config
	ConfigVolume = "config-data"

	// Path to deploy the Prometheus CaCert if needed
	PrometheusCaCertFolderPath = "/etc/pki/ca-trust/extracted/pem/prometheus/"

	// WatcherUserID is the linux user ID used by Kolla for watcher service
	WatcherUserID int64 = 42451

	// scriptVolume is the name of the volume used to ship scripts into pods
	scriptVolume = "scripts-volume"
)
