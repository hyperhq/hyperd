#!/bin/bash

PROJECT=$(readlink -f $(dirname $0)/../..)
FEDORA_DIR=${PROJECT}/package/fedora
VERSION=0.5

if [ $# -gt 0 ] ; then
    VERSION=$1
fi

#SOURCES

cd $PROJECT
git archive --format=tar.gz master > ${FEDORA_DIR}/rpm/SOURCES/hyper-${VERSION}.tar.gz
cd $PROJECT/../runv
git archive --format=tar.gz master > ${FEDORA_DIR}/rpm/SOURCES/runv-${VERSION}.tar.gz
cd $PROJECT/../hyperstart
git archive --format=tar.gz master > ${FEDORA_DIR}/rpm/SOURCES/hyperstart-${VERSION}.tar.gz
cd ..
[ -d qboot ] && rm -rf qboot
git clone https://github.com/bonzini/qboot.git
cd qboot
git archive --format=tar.gz master > ${FEDORA_DIR}/rpm/SOURCES/qboot.tar.gz


sed -e "s#%PROJECT_ROOT%#${PROJECT}#g" ${FEDORA_DIR}/fedora-rpm.pod.in > ${FEDORA_DIR}/fedora-rpm.pod

${PROJECT}/hyper run -a --rm -p ${FEDORA_DIR}/fedora-rpm.pod

