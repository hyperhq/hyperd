#!/bin/bash

PROJECT=$(readlink -f $(dirname $0)/../..)
DEBIAN_DIR=${PROJECT}/package/debian

${DEBIAN_DIR}/hypercontainer/make-hypercontainer-deb.sh "$@"
${DEBIAN_DIR}/hyperstart/make-hyperstart-deb.sh "$@"
