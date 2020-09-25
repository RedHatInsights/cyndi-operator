oc whoami || exit 1

oc create ns kafka
oc create ns my-kafka-project

oc apply -f install/cluster-operator/ -n kafka

oc apply -f install/cluster-operator/020-RoleBinding-strimzi-cluster-operator.yaml -n my-kafka-project
oc apply -f install/cluster-operator/032-RoleBinding-strimzi-cluster-operator-topic-operator-delegation.yaml -n my-kafka-project
oc apply -f install/cluster-operator/031-RoleBinding-strimzi-cluster-operator-entity-operator-delegation.yaml -n my-kafka-project

oc project my-kafka-project

oc create -n my-kafka-project -f cluster.yml
oc wait kafka/my-cluster --for=condition=Ready --timeout=300s -n my-kafka-project

oc apply -f inventory-db.secret.yml -n my-kafka-project
oc apply -f advisor-db-init.yml -n my-kafka-project
oc apply -f inventory-db.yaml -n my-kafka-project

oc wait deployment/inventory-db --for=condition=Available --timeout=300s -n my-kafka-project

oc apply -f inventory-mq.yml -n my-kafka-project
oc apply -f inventory-api.yml -n my-kafka-project

oc wait dc/inventory-mq-pmin --for=condition=Available --timeout=300s -n my-kafka-project
oc wait deployment/insights-inventory --for=condition=Available --timeout=300s -n my-kafka-project

oc apply -f advisor-db.secret.yml -n my-kafka-project
oc apply -f advisor-db.yaml -n my-kafka-project
oc wait deployment/advisor-db --for=condition=Available --timeout=300s -n my-kafka-project

oc apply -f kafka-connect.yaml -n my-kafka-project
oc wait kafkaconnect/my-connect-cluster --for=condition=Ready --timeout=300s -n my-kafka-project




