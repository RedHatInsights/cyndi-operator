#!/bin/bash

set -exv

IMAGE="quay.io/cloudservices/cyndi-operator"
IMAGE_TAG=$(git rev-parse --short=7 HEAD)
SECURITY_COMPLIANCE_TAG="sc-$(date +%Y%m%d)-$(git rev-parse --short=7 HEAD)"

if [[ -z "$QUAY_USER" || -z "$QUAY_TOKEN" ]]; then
    echo "QUAY_USER and QUAY_TOKEN must be set"
    exit 1
fi

docker login -u="$QUAY_USER" -p="$QUAY_TOKEN" quay.io

# Check if the multiarchbuilder exists
if docker buildx ls | grep -q "multiarchbuilder"; then
    echo "Using multiarchbuilder for buildx"
    # Multi-architecture build
    docker buildx use multiarchbuilder
    docker buildx build --platform linux/amd64,linux/arm64 -t "${IMAGE}:${IMAGE_TAG}" --push .
else
    echo "Falling back to standard build and push"
    # Standard build and push
    docker build -t "${IMAGE}:${IMAGE_TAG}" .
    docker push "${IMAGE}:${IMAGE_TAG}"
fi

if [[ "$GIT_BRANCH" == "origin/security-compliance" ]]; then
    docker build -t "${IMAGE}:${IMAGE_TAG}" .
    docker  tag "${IMAGE}:${IMAGE_TAG}" "${IMAGE}:${SECURITY_COMPLIANCE_TAG}"
    docker  push "${IMAGE}:${SECURITY_COMPLIANCE_TAG}"
fi
