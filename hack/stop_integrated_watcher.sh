#!/bin/bash
set -x

WATCHER_CSV_NAME="$(oc get csv -n openstack-operators -l operators.coreos.com/watcher-operator.openstack-operators -o name)"

if [ -z ${WATCHER_CSV_NAME} ]; then
    OPENSTACK_CSV_NAME="$(oc get csv -n openstack-operators -l operators.coreos.com/openstack-operator.openstack-operators -o name)"
    if [ -n "{OPENSTACK_CSV_NAME}" ]; then
        WATCHER_POD_NAME="$(oc get pod -n openstack-operators -o name | grep watcher-operator-controller-manager || true)"
        OPENSTACK_OPERATOR_POD_NAME="$(oc get pod -n openstack-operators -o name | grep openstack-operator-controller-operator || true)"
        oc patch "${OPENSTACK_CSV_NAME}" -n openstack-operators --type=json -p="[{'op': 'replace', 'path': '/spec/install/spec/deployments/0/spec/replicas', 'value': 0}]"
        if [ -n "${OPENSTACK_OPERATOR_POD_NAME}" ]; then
            oc wait --for=delete "${OPENSTACK_OPERATOR_POD_NAME}" -n openstack-operators --timeout=60s
        fi
        oc scale --replicas=0 -n openstack-operators deploy/watcher-operator-controller-manager
        if [ -n "${WATCHER_POD_NAME}" ]; then
            oc wait --for=delete "${WATCHER_POD_NAME}" -n openstack-operators --timeout=60s
        fi
    else
        echo "Openstack operator is not installed"
    fi
else
    echo "Watcher operator installed in standalone mode"
fi
