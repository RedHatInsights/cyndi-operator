#!/bin/bash

pkill -f "kubectl port-forward"

PROJECT_NAME=$1

if [ -z "$PROJECT_NAME" ]; then
  PROJECT_NAME="test"
fi

echo "Using namespace $PROJECT_NAME"

KAFKA_NAME=$(kubectl get kafka -o custom-columns=:metadata.name -n "$PROJECT_NAME" | xargs)
CONNECT_NAME=$(kubectl get kafkaconnect -o custom-columns=:metadata.name -n "$PROJECT_NAME" | xargs)
KAFKA_SVC="svc/$KAFKA_NAME-kafka-bootstrap"
CONNECT_SVC="svc/$CONNECT_NAME-connect-api"
ELASTICSEARCH_SVC="svc/xjoin-elasticsearch-es-default"
HBI_DB_SVC="svc/host-inventory-db"
XJOIN_SVC="svc/xjoin-search"
HBI_SVC="svc/host-inventory-service"
APICURIO_SVC="svc/xjoin-apicurio-service"
XJOIN_API_GATEWAY="svc/xjoin-api-gateway-api"
ADVISOR_DB_SVC="svc/advisor-backend-db"

kubectl port-forward "$ADVISOR_DB_SVC" 5433:5432 -n "$PROJECT_NAME" >/dev/null 2>&1 &
kubectl port-forward "$HBI_DB_SVC" 5432:5432 -n "$PROJECT_NAME" >/dev/null 2>&1 &
kubectl port-forward "$CONNECT_SVC" 8083:8083 -n "$PROJECT_NAME" >/dev/null 2>&1 &
kubectl port-forward "$ELASTICSEARCH_SVC" 9200:9200 -n "$PROJECT_NAME" >/dev/null 2>&1 &
kubectl port-forward "$KAFKA_SVC" 9092:9092 -n "$PROJECT_NAME" >/dev/null 2>&1 &
kubectl port-forward "$KAFKA_SVC" 29092:9092 -n "$PROJECT_NAME" >/dev/null 2>&1 &
#kubectl port-forward "$XJOIN_SVC" 4000:4000 -n "$PROJECT_NAME" >/dev/null 2>&1 &
kubectl port-forward "$HBI_SVC" 8000:8000 -n "$PROJECT_NAME" >/dev/null 2>&1 &
kubectl port-forward "$APICURIO_SVC" 10001:10000 -n "$PROJECT_NAME" >/dev/null 2>&1 &
kubectl port-forward "$XJOIN_API_GATEWAY" 10000:10000 -n "$PROJECT_NAME" >/dev/null 2>&1 &
kubectl port-forward "$CONNECT_SVC" 5005:5005 -n "$PROJECT_NAME" >/dev/null 2>&1 &

pgrep -fla "kubectl port-forward"
