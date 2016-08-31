#!/bin/bash

PROJECT=$(readlink -f $(dirname $0)/../..)
FEDORA_DIR=${PROJECT}/package/fedora
VERSION=0.6.2

if [ $# -gt 0 ] ; then
    VERSION=$1
fi

#SOURCES

cd $PROJECT
git archive --format=tar.gz master > ${FEDORA_DIR}/rpm/SOURCES/hyper-container-${VERSION}.tar.gz
cd $PROJECT/../runv
git archive --format=tar.gz master > ${FEDORA_DIR}/rpm/SOURCES/runv-${VERSION}.tar.gz
cd $PROJECT/../hyperstart
git archive --format=tar.gz master > ${FEDORA_DIR}/rpm/SOURCES/hyperstart-${VERSION}.tar.gz


sed -e "s#%PROJECT_ROOT%#${PROJECT}#g" ${FEDORA_DIR}/fedora-rpm.pod.in > ${FEDORA_DIR}/fedora-rpm.pod

hyperctl run -a --rm -p ${FEDORA_DIR}/fedora-rpm.pod

