#!/usr/bin/env sh

set -o errexit
set -o nounset

umask 077

# Automatically get the folder where the source files must be placed.
__DIR=$(dirname "${0}")
SOURCE_DIR="${__DIR}/../"
TAG_VALUE="${1}"
TAG_VALUE_WITHOUT_V="$(echo ${TAG_VALUE} | sed s/^v//)"

# Create a variable that contains the current date in UTC
# Different flow if this script is running on Darwin or Linux machines.
if [ "$(uname)" = "Darwin" ]; then
  NOW_DATE="$(date -u +%F)"
else
  NOW_DATE="$(date -u -I)"
fi

sed -i.bck -E "s|^VERSION \?= [0-9]+.[0-9]+.[0-9]+.*|VERSION ?= ${TAG_VALUE_WITHOUT_V}|" "${SOURCE_DIR}/Makefile"
sed -i.bck -E "s|newTag: [0-9]+.[0-9]+.[0-9]+.*|newTag: ${TAG_VALUE_WITHOUT_V}|" "${SOURCE_DIR}/config/manager/kustomization.yaml"
sed -i.bck -E "s|containerImage:(.*)kubegreen/kube-green:[0-9]+.[0-9]+.[0-9]+.*|containerImage:\1kubegreen/kube-green:${TAG_VALUE_WITHOUT_V}|" "${SOURCE_DIR}/config/manifests/bases/kube-green.clusterserviceversion.yaml"
sed -i.bck -E "s|replaces: (.*)|replaces:$(grep 'name: kube-green\.v' ./bundle/manifests/kube-green.clusterserviceversion.yaml | cut -d ':' -f2)|" ${SOURCE_DIR}/config/manifests/bases/kube-green.clusterserviceversion.yaml
sed -i.bck -E "/manager:/,/tag:/s/tag: \"[0-9]+.[0-9]+.[0-9]+.*\"/tag: \"$TAG_VALUE_WITHOUT_V\"/" ${SOURCE_DIR}/charts/kube-green/values.yaml
sed -i.bck -E "s|appVersion: \"[0-9]+.[0-9]+.[0-9]+.*\"|appVersion: \"$TAG_VALUE_WITHOUT_V\"|" ${SOURCE_DIR}/charts/kube-green/Chart.yaml
sed -i.bck -E "s|version: [0-9]+.[0-9]+.[0-9]+.*|version: $TAG_VALUE_WITHOUT_V|" ${SOURCE_DIR}/charts/kube-green/Chart.yaml
rm -fr \
  "${SOURCE_DIR}/Makefile.bck" \
  "${SOURCE_DIR}/config/manager/kustomization.yaml.bck" \
  "${SOURCE_DIR}/config/manifests/bases/kube-green.clusterserviceversion.yaml.bck" \
  "${SOURCE_DIR}/charts/kube-green/values.yaml.bck" \
  "${SOURCE_DIR}/charts/kube-green/Chart.yaml.bck"
