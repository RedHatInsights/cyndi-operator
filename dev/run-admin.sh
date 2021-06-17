if [[ -z "$1" ]]; then
    echo "pull secret name must be set"
    exit 1
fi

PULL_SECRET=$1

oc whoami || exit 1
oc get secret $1 -n cyndi || exit 1

oc secrets link -n cyndi default $PULL_SECRET --for=pull

oc create ns kafka

oc project cyndi

oc apply -f amq-streams.yaml
oc wait subscription amq-streams --for=condition=CatalogSourcesUnhealthy=false -n openshift-operators
oc wait crd -l app=strimzi --for=condition=Established

oc apply -n cyndi -f cluster.yml
oc wait kafka/my-cluster --for=condition=Ready --timeout=300s -n cyndi

oc apply -f inventory-db.secret.yml -n cyndi
oc apply -f advisor-db-init.yml -n cyndi
oc apply -f inventory-db.yaml -n cyndi

oc wait deployment/inventory-db --for=condition=Available --timeout=300s -n cyndi

oc apply -f inventory-mq.yml -n cyndi
oc apply -f inventory-api.yml -n cyndi

oc wait dc/inventory-mq-pmin --for=condition=Available --timeout=300s -n cyndi
oc wait deployment/insights-inventory --for=condition=Available --timeout=300s -n cyndi

oc apply -f advisor-db.secret.yml -n cyndi
oc apply -f advisor-db.yaml -n cyndi
oc wait deployment/advisor-db --for=condition=Available --timeout=300s -n cyndi

oc apply -f kafka-user.yaml -n cyndi
oc wait kafkauser/kafka-cyndi --for=condition=Ready --timeout=300s -n cyndi

oc apply -f kafka-topics.yaml
oc wait kafkatopic/platform.cyndi.dlq --for=condition=Ready --timeout=300s -n cyndi

oc apply -f kafka-connect.yaml -n cyndi
oc wait kafkaconnect/xjoin-kafka-connect-strimzi --for=condition=Ready --timeout=300s -n cyndi






