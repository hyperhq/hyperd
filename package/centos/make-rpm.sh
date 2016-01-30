#!/bin/bash

PROJECT=$(readlink -f $(dirname $0)/../..)
CENTOS_DIR=${PROJECT}/package/centos
VERSION=0.4

if [ $# -gt 0 ] ; then
    VERSION=$1
fi

#SOURCES

cd $PROJECT
git archive --format=tar.gz master > ${CENTOS_DIR}/rpm/SOURCES/hyper-${VERSION}.tar.gz
cd $PROJECT/../runv
git archive --format=tar.gz master > ${CENTOS_DIR}/rpm/SOURCES/runv-${VERSION}.tar.gz
cd $PROJECT/../hyperstart
git archive --format=tar.gz master > ${CENTOS_DIR}/rpm/SOURCES/hyperstart-${VERSION}.tar.gz
cd ..
[ -d qboot ] && rm -rf qboot
git clone https://github.com/bonzini/qboot.git
cd qboot
git archive --format=tar.gz master > ${CENTOS_DIR}/rpm/SOURCES/qboot.tar.gz
curl -sSL http://wiki.qemu-project.org/download/qemu-2.4.1.tar.bz2 > ${CENTOS_DIR}/rpm/SOURCES/qemu-2.4.1.tar.bz2


sed -e "s#%PROJECT_ROOT%#${PROJECT}#g" ${CENTOS_DIR}/centos-rpm.pod.in > ${CENTOS_DIR}/centos-rpm.pod
sed -e "s#%VERSION%#${VERSION}#g" ${CENTOS_DIR}/rpm/SPECS/hyper.spec.in > ${CENTOS_DIR}/rpm/SPECS/hyper.spec
sed -e "s#%VERSION%#${VERSION}#g" ${CENTOS_DIR}/rpm/SPECS/hyperstart.spec.in > ${CENTOS_DIR}/rpm/SPECS/hyperstart.spec

${PROJECT}/hyper run -a --rm -p ${CENTOS_DIR}/centos-rpm.pod

