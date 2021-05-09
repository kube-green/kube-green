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

sed -i.bck "s|## Unreleased|## v${TAG_VALUE} - ${NOW_DATE}|g" "${SOURCE_DIR}/CHANGELOG.md"
sed -i.bck "s|VERSION ?= [0-9]*.[0-9]*.[0-9]*.*|VERSION ?= ${TAG_VALUE_WITHOUT_V}|" "${SOURCE_DIR}/Makefile"
rm -fr "${SOURCE_DIR}/CHANGELOG.md.bck" "${SOURCE_DIR}/Makefile.bck"
