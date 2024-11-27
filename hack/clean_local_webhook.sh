#!/bin/bash
set -ex

oc delete validatingwebhookconfiguration/vwatcher.kb.io --ignore-not-found
oc delete mutatingwebhookconfiguration/mwatcher.kb.io --ignore-not-found
oc delete validatingwebhookconfiguration/vwatcherapi.kb.io --ignore-not-found
oc delete mutatingwebhookconfiguration/mwatcherapi.kb.io --ignore-not-found
oc delete validatingwebhookconfiguration/vwatcherdecisionengine.kb.io --ignore-not-found
oc delete mutatingwebhookconfiguration/mwatcherdecisionengine.kb.io --ignore-not-found
oc delete validatingwebhookconfiguration/vwatcherapplier.kb.io --ignore-not-found
oc delete mutatingwebhookconfiguration/mwatcherapplier.kb.io --ignore-not-found
