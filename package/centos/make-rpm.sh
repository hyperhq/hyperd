#!/bin/bash

PROJECT=$(readlink -f $(dirname $0)/../..)
CENTOS_DIR=${PROJECT}/package/centos
VERSION=0.6.2

if [ $# -gt 0 ] ; then
    VERSION=$1
fi

#SOURCES

cd $PROJECT
git archive --format=tar.gz master > ${CENTOS_DIR}/rpm/SOURCES/hyperd-${VERSION}.tar.gz
cd $PROJECT/../runv
git archive --format=tar.gz master > ${CENTOS_DIR}/rpm/SOURCES/runv-${VERSION}.tar.gz
cd $PROJECT/../hyperstart
git archive --format=tar.gz master > ${CENTOS_DIR}/rpm/SOURCES/hyperstart-${VERSION}.tar.gz

sed -e "s#%PROJECT_ROOT%#${PROJECT}#g" ${CENTOS_DIR}/centos-rpm.pod.in > ${CENTOS_DIR}/centos-rpm.pod

hyperctl run -a --rm -p ${CENTOS_DIR}/centos-rpm.pod

