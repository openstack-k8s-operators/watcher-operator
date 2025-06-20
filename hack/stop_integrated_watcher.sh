#!/bin/bash
set -x

WATCHER_CSV_NAME="$(oc get csv -n openstack-operators -l operators.coreos.com/watcher-operator.openstack-operators -o name)"

if [ -z ${WATCHER_CSV_NAME} ]; then
    OPENSTACK_CSV_NAME="$(oc get csv -n openstack-operators -l operators.coreos.com/openstack-operator.openstack-operators -o name)"
    if [ -n "{OPENSTACK_CSV_NAME}" ]; then
        oc patch "${OPENSTACK_CSV_NAME}" -n openstack-operators --type=json -p="[{'op': 'replace', 'path': '/spec/install/spec/deployments/0/spec/replicas', 'value': 0}]"
        oc scale --replicas=0 -n openstack-operators deploy/watcher-operator-controller-manager
    else
        echo "Openstack operator is not installed"
    fi
else
    echo "Watcher operator installed in standalone mode"
fi
