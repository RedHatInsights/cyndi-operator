#!/bin/bash

#
# This is a helper script to deploy to crc during development
#

oc whoami >> /dev/null 2>&1
if [ $? == 1 ];
then
    echo "You must login to Openshift first."
    echo "https://oauth-openshift.apps-crc.testing/oauth/token/display"
    exit 1
fi

operator-sdk build default-route-openshift-image-registry.apps-crc.testing/default/cyndi-operator:v0.0.1

if [ $? != 1 ];
then
    docker login -u kubeadmin -p $(oc whoami -t) default-route-openshift-image-registry.apps-crc.testing
    docker push default-route-openshift-image-registry.apps-crc.testing/default/cyndi-operator:v0.0.1
    oc delete CyndiPipeline/example-cyndipipeline
    oc delete ClusterServiceVersion/cyndi-operator.v0.0.1
    oc apply -f deploy/olm-catalog/cyndi-operator/manifests/cyndi-operator.clusterserviceversion.yaml
    oc apply -f deploy/crds/cyndi.cloud.redhat.com_cyndipipelines_crd.yaml
    oc create -f deploy/crds/cyndi.cloud.redhat.com_v1beta1_cyndipipeline_cr.yaml
    sleep 10
    oc logs -f Deployment/cyndi-operator
fi
