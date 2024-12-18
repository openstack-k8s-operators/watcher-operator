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

	// CustomConfigFileName - File name with custom configuration
	CustomConfigFileName = "01-custom.conf"

	// LogVolume is the default logVolume name used to mount logs
	LogVolume = "logs"
)
