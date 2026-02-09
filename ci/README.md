# Continuous Integration

This document describes the CI jobs used to test and validate the watcher-operator.

## CI Jobs Matrix

The following table shows the different CI jobs and their configuration:

| Job Name | OpenStack Version | OCP Version | Notifications | App Credentials | NFS Backend |
|----------|-------------------|-------------|---------------|-----------------|-------------|
| `watcher-operator-validation-master` | master | 4.18 | ✅ Yes | ✅ Yes | ✅ Yes |
| `watcher-operator-validation-epoxy` | antelope (all) - epoxy (watcher) | 4.18 | ❌ No | ❌ No | ❌ No |
| `watcher-operator-validation-epoxy-ocp4-16` | antelope (all) - epoxy (watcher) | 4.16 | ❌ No | ❌ No | ❌ No |
| `watcher-operator-kuttl` | master | 4.18 | N/A (unit tests) | ❌ No | ❌ No |
| `periodic-watcher-operator-validation-master` | master | 4.18 | ✅ Yes | ✅ Yes | ❌ No |

## Job Descriptions

### watcher-operator-kuttl
Runs kuttl tests for the operator, including Application Credentials rotation tests. This job does not deploy a full EDPM environment.

### watcher-operator-validation-master
Validates watcher-operator with master OpenStack content on OCP 4.18. Uses Application Credentials for authentication. Configures notifications over a dedicated RabbitMQ instance. Includes NFS backend for Cinder.

### watcher-operator-validation-epoxy
Validates watcher-operator with epoxy OpenStack release. Does not enable notifications dedicated RabbitMQ instance or Application Credentials.

### watcher-operator-validation-epoxy-ocp4-16
Qualification job for epoxy release on OCP 4.16. Does not enable notifications or Application Credentials.

### periodic-watcher-operator-validation-master
Periodic job that runs in the RDO master promotion pipeline with same configuration than watcher-operator-validation-master

## Scenarios

CI scenarios are defined in `ci/scenarios/` directory:

- **edpm.yml**: Base scenario with notifications enabled
- **edpm-no-notifications.yml**: Scenario without notificationss, with reduced model collection period (60s)
- **kuttl.yml**: Scenario for kuttl unit tests
- **nfs.yml**: Additional configuration for NFS backend

## Adding New Jobs

When adding new CI jobs, follow the following patterns:
1. Create a complete scenario file in `ci/scenarios/` with all necessary configuration
2. Define the job in `.zuul.yaml` inheriting from the appropriate base job
3. Add the job to the relevant project template
4. Update this README with the new job details
