#!/bin/bash

PROJECT=$(readlink -f $(dirname $0)/../..)
UBUNTU_DIR=${PROJECT}/package/ubuntu

${UBUNTU_DIR}/hypercontainer/make-hypercontainer-deb.sh
${UBUNTU_DIR}/hyperstart/make-hyperstart-deb.sh

