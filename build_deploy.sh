#!/bin/bash

set -exv

IMAGE="quay.io/cloudservices/cyndi-operator"
IMAGE_TAG=$(git rev-parse --short=7 HEAD)

if [[ -z "$QUAY_USER" || -z "$QUAY_TOKEN" ]]; then
    echo "QUAY_USER and QUAY_TOKEN must be set"
    exit 1
fi

DOCKER_CONF="$PWD/.docker"
mkdir -p "$DOCKER_CONF"
docker --config="$DOCKER_CONF" login -u="$QUAY_USER" -p="$QUAY_TOKEN" quay.io


docker --config="$DOCKER_CONF" build --platform="linux/amd64" -t "${IMAGE}:${IMAGE_TAG}-amd64" --push .
docker --config="$DOCKER_CONF" build --platform="linux/arm64" -t "${IMAGE}:${IMAGE_TAG}-arm64" --push .

docker --config="$DOCKER_CONF" manifest create "${IMAGE}:${IMAGE_TAG}" \
    "${IMAGE}:${IMAGE_TAG}-amd64" \
    "${IMAGE}:${IMAGE_TAG}-arm64"

docker --config="$DOCKER_CONF" manifest push "${IMAGE}:${IMAGE_TAG}"
