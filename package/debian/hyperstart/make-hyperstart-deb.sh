#!/bin/bash

PROJECT=$(readlink -f $(dirname $0)/../../..)
DEBIAN_DIR=${PROJECT}/package/debian/hyperstart
VERSION=${VERSION:-0.8.1}
BRANCH=${BRANCH:-master}

if [ $# -gt 0 ] ; then
    VERSION=$1
fi

# install addtional pkgs in order to build deb pkg
# sudo apt-get install -y autoconf automake pkg-config dh-make

# get hyperstart tar ball
cd $PROJECT/../hyperstart
git archive --format=tar.gz ${BRANCH} > ${DEBIAN_DIR}/hyperstart-${VERSION}.tar.gz

# prepair to create source pkg
mkdir -p ${DEBIAN_DIR}/hyperstart-${VERSION}
cd ${DEBIAN_DIR}
tar -zxf hyperstart-${VERSION}.tar.gz -C ${DEBIAN_DIR}/hyperstart-${VERSION}

# in order to use debian/* to create deb, so put them in the hyperstart.
cp -a ${DEBIAN_DIR}/debian ${DEBIAN_DIR}/hyperstart-${VERSION}

# run dh_make
cd ${DEBIAN_DIR}/hyperstart-${VERSION}
dh_make -s -y -f ../hyperstart_${VERSION}.orig.tar.gz -e dev@hyper.sh

# run dpkg-buildpackage
dpkg-buildpackage -b -us -uc -rfakeroot

#clean up intermediate files
rm -rf ${DEBIAN_DIR}/hyperstart-${VERSION}

