#!/bin/bash

# --------------------------------------------
# Options that must be configured by app owner
# --------------------------------------------
APP_NAME="cyndi"  # name of app-sre "application" folder this component lives in
COMPONENT_NAME="cyndi-operator"  # name of app-sre "resourceTemplate" in deploy.yaml for this component
IMAGE="quay.io/cloudservices/cyndi-operator"  # image location on quay

IQE_PLUGINS=pr_check"cyndi"  # name of the IQE plugin for this app.
IQE_MARKER_EXPRESSION="smoke"  # This is the value passed to pytest -m
IQE_FILTER_EXPRESSION=""  # This is the value passed to pytest -k
IQE_CJI_TIMEOUT="30m"  # This is the time to wait for smoke test to complete or fail

DOCKERFILE="Dockerfile"

# Install bonfire repo/initialize
# https://raw.githubusercontent.com/RedHatInsights/bonfire/master/cicd/bootstrap.sh
# This script automates the install / config of bonfire
CICD_URL=https://raw.githubusercontent.com/RedHatInsights/bonfire/master/cicd
curl -s $CICD_URL/bootstrap.sh > .cicd_bootstrap.sh && source .cicd_bootstrap.sh

# This script is used to build the image that is used in the PR Check
source $CICD_ROOT/build.sh

# this script runs unit tests.
source $APP_ROOT/unit_test.sh

# the below part works.
ARTIFACTS_DIR=$WORKSPACE/artifacts

mkdir -p $ARTIFACTS_DIR
cat << EOF > $ARTIFACTS_DIR/junit-dummy.xml
<testsuite tests="1">
    <testcase classname="dummy" name="dummytest"/>
</testsuite>
EOF