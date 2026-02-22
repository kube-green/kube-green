#!/usr/bin/env bash

# Update FBC catalog template with a new bundle version.
#
# Usage:
#   ./hack/update-catalog-template.sh <catalog_template> <version> [previous_version]
#
# Examples:
#   ./hack/update-catalog-template.sh config/catalog/catalog-template.yaml 0.8.0
#   ./hack/update-catalog-template.sh config/catalog/catalog-template.yaml 0.8.0 0.7.1

set -euo pipefail

CATALOG_TEMPLATE="${1:-}"
VERSION="${2:-}"
PREVIOUS_VERSION="${3:-}"

if [[ -z "$CATALOG_TEMPLATE" ]] || [[ -z "$VERSION" ]]; then
    echo "Usage: $0 <catalog_template> <version> [previous_version]"
    exit 1
fi

PACKAGE_NAME="kube-green"
BUNDLE_IMAGE="${BUNDLE_IMAGE:-docker.io/kubegreen/kube-green-bundle:v${VERSION}}"

# Add bundle entry
yq e -i ".entries += [{\"image\": \"${BUNDLE_IMAGE}\", \"name\": \"${PACKAGE_NAME}.v${VERSION}\", \"schema\": \"olm.bundle\"}]" "${CATALOG_TEMPLATE}"

# Add to channel entries
if [[ -n "$PREVIOUS_VERSION" ]]; then
    yq e -i "(.entries[] | select(.schema == \"olm.channel\")).entries += [{\"name\": \"${PACKAGE_NAME}.v${VERSION}\", \"replaces\": \"${PACKAGE_NAME}.v${PREVIOUS_VERSION}\"}]" "${CATALOG_TEMPLATE}"
else
    yq e -i "(.entries[] | select(.schema == \"olm.channel\")).entries += [{\"name\": \"${PACKAGE_NAME}.v${VERSION}\"}]" "${CATALOG_TEMPLATE}"
fi

echo "Updated ${CATALOG_TEMPLATE} with version ${VERSION}"
