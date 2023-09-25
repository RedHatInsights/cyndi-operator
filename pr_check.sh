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

which qemu-user-static
cat /proc/sys/fs/binfmt_misc/qemu-*
docker version
docker buildx version

docker buildx build --platform linux/arm64 -t "${IMAGE}:${IMAGE_TAG}-arm64" .
docker buildx build --platform linux/amd64 -t "${IMAGE}:${IMAGE_TAG}-amd64" .

