#!/bin/bash
set -e

##########################################################################
# This is a copypasta from:
# https://github.com/app-sre/deployment-validation-operator/blob/master/build_catalog.sh
#
##########################################################################

function log ()
{
  echo "######## $1 ########"
}

count=0
for var in BUNDLE_IMAGE \
           CATALOG_IMAGE \
           QUAY_USER \
           QUAY_TOKEN
do
  if [ ! "${!var}" ]; then
    log "$var is not set"
    count=$((count + 1))
  fi
done

[ $count -gt 0 ] && exit 1

function build_a_tag ()
{
  tag=$1
  echo "Building tag: $tag"

  num_commits=$(git rev-list $(git rev-list --max-parents=0 HEAD)..HEAD --count)
  current_commit=$(git rev-parse --short=7 HEAD)
  version="0.1.$num_commits-git$current_commit"
  opm_version="1.24.0"

  # Download opm build
  curl -L https://github.com/operator-framework/operator-registry/releases/download/v$opm_version/linux-amd64-opm -o ./opm
  chmod u+x ./opm

  # workaround for https://github.com/golang/go/issues/38373
  GO_VERSION="1.17.12"
  GOUNPACK=$(mktemp -d)
  wget -q "https://golang.org/dl/go${GO_VERSION}.linux-amd64.tar.gz" -O $GOUNPACK/go.tar.gz
  tar -C $GOUNPACK -xzf $GOUNPACK/go.tar.gz
  export PATH=${GOUNPACK}/go/bin:$PATH

  # Login to docker
  export DOCKER_CONFIG="$PWD/.docker"
  mkdir -p "$DOCKER_CONFIG"

  docker login -u="$QUAY_USER" -p="$QUAY_TOKEN" quay.io

  # Find the CSV version from the previous bundle
  log "Pulling $tag bundle image $BUNDLE_IMAGE"
  docker pull $BUNDLE_IMAGE:$tag && exists=1 || exists=0

  if [ $exists -eq 1 ]; then
    log "Extracting previous version from bundle image"
    docker create --name="tmp_$$" $BUNDLE_IMAGE:$tag sh
    tmp_dir=$(mktemp -d -t sa-XXXXXXXXXX)
    pushd $tmp_dir
      docker export tmp_$$ | tar -xf -
      prev_version=`find . -name *.clusterserviceversion.* | xargs cat - | python3 -c 'import sys,yaml; print(yaml.safe_load(sys.stdin.read())["spec"]["version"])'`
      if [[ "$prev_version" == "" ]]; then
        log "Unable to find previous bundle version"
        exit 1
      fi
      log "Found previous bundle version $prev_version"
    popd
    rm -rf $tmp_dir
    docker rm tmp_$$

    ###############################
    #Uncomment to reset the catalog
    ###############################
    #log "Resetting index"
    ./opm index prune -f $CATALOG_IMAGE:$tag -c docker --tag $CATALOG_IMAGE:$tag -f $CATALOG_IMAGE -p blank
    ./opm index prune-stranded -f $CATALOG_IMAGE:$tag -c docker --tag $CATALOG_IMAGE:$tag
    ./opm index rm -f $CATALOG_IMAGE:$tag -c docker --tag $CATALOG_IMAGE:$tag -o cyndi
    docker push $CATALOG_IMAGE:$tag
    export SKIP_VERSION=$version
    prev_version=""
    unset REPLACE_VERSION
  fi

  # Build/push the new bundle
  log "Creating bundle $BUNDLE_IMAGE:$current_commit"
  if [[ $prev_version != "" ]]; then
    log "Setting REPLACE_VERSION"
    export REPLACE_VERSION=$prev_version
  fi
  export BUNDLE_IMAGE_TAG=$current_commit
  export VERSION=$version
  curl -L https://github.com/operator-framework/operator-sdk/releases/download/v1.12.0/operator-sdk_linux_amd64 -o ./operator-sdk
  curl -LO https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2Fv4.3.0/kustomize_v4.3.0_linux_amd64.tar.gz
  curl -L https://github.com/mikefarah/yq/releases/download/v4.13.4/yq_linux_amd64 -o ./yq
  tar -xvf kustomize_v4.3.0_linux_amd64.tar.gz
  chmod +x ./yq
  chmod +x ./operator-sdk
  chmod +x ./kustomize
  export PATH=$PATH:.
  make bundle-build
  docker tag $BUNDLE_IMAGE:$current_commit $BUNDLE_IMAGE:$tag

  log "Pushing the bundle $BUNDLE_IMAGE:$current_commit to repository"
  docker push $BUNDLE_IMAGE:$current_commit
  # Do not push the latest tag here.  If there is a problem creating the catalog then
  # pushing the latest tag here will mean subsequent runs will be extracting a bundle
  # version that isn't referenced in the catalog.  This will result in all future
  # catalog creation failing to be created.

  # Create/push a new catalog via opm
  log "Pulling existing $tag catalog $CATALOG_IMAGE"
  docker pull $CATALOG_IMAGE:$tag && exists=1 || exists=0
  if [ $exists -eq 1 ]; then
    from_arg="--from-index $CATALOG_IMAGE:$tag"
  fi

  if [[ "$from_arg" == "" ]]; then
    log "Creating new catalog $CATALOG_IMAGE"
  else
    log "Updating existing catalog $CATALOG_IMAGE"
  fi

  ./opm index add --bundles $BUNDLE_IMAGE:$current_commit $from_arg --tag $CATALOG_IMAGE:$current_commit --build-tool docker
  if [ $? -ne 0 ]; then
    exit 1
  fi
  docker tag $CATALOG_IMAGE:$current_commit $CATALOG_IMAGE:$tag

  log "Pushing catalog $CATALOG_IMAGE:$current_commit to repository"
  docker push $CATALOG_IMAGE:$current_commit

  # Only put the $tag tags once everything else has succeeded
  log "Pushing $tag tags for $BUNDLE_IMAGE and $CATALOG_IMAGE"
  docker push $CATALOG_IMAGE:$tag
  docker push $BUNDLE_IMAGE:$tag
}
