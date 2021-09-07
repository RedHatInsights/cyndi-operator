#!/bin/bash

pkill -f "kubectl port-forward"

PROJECT_NAME=$1

if [ -z "$PROJECT_NAME" ]; then
  PROJECT_NAME="cyndi"
fi

echo "Using namespace $PROJECT_NAME"

ADVISOR_DB_SVC="svc/advisor-db"
HBI_DB_SVC="svc/inventory-db"
HBI_SVC="svc/insights-inventory"
KAFKA_SVC="svc/my-cluster-kafka-bootstrap"
CONNECT_SVC="svc/xjoin-kafka-connect-strimzi-connect-api"

kubectl port-forward "$HBI_DB_SVC" 5432:5432 -n "$PROJECT_NAME" >/dev/null 2>&1 &
kubectl port-forward "$CONNECT_SVC" 8083:8083 -n "$PROJECT_NAME" >/dev/null 2>&1 &
kubectl port-forward "$KAFKA_SVC" 9092:9092 -n "$PROJECT_NAME" >/dev/null 2>&1 &
kubectl port-forward "$KAFKA_SVC" 29092:9092 -n "$PROJECT_NAME" >/dev/null 2>&1 &
kubectl port-forward "$HBI_SVC" 8080:8080 -n "$PROJECT_NAME" >/dev/null 2>&1 &
kubectl port-forward "$ADVISOR_DB_SVC" 5433:5432 -n "$PROJECT_NAME" >/dev/null 2>&1 &


pgrep -fla "kubectl port-forward"
