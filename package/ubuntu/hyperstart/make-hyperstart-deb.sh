#!/bin/bash

PROJECT=$(readlink -f $(dirname $0)/../../..)
UBUNTU_DIR=${PROJECT}/package/ubuntu/hyperstart
VERSION=0.6.2

if [ $# -gt 0 ] ; then
    VERSION=$1
fi

# install addtional pkgs in order to build deb pkg
sudo apt-get install -y autoconf automake pkg-config

# get hyperstart tar ball
cd $PROJECT/../hyperstart
git archive --format=tar.gz master > ${UBUNTU_DIR}/hyperstart-${VERSION}.tar.gz

# prepair to create source pkg
mkdir -p ${UBUNTU_DIR}/hyperstart-${VERSION}
cd ${UBUNTU_DIR}
tar -zxvf hyperstart-${VERSION}.tar.gz -C ${UBUNTU_DIR}/hyperstart-${VERSION}

# in order to use debian/* to create deb, so put them in the hyperstart.
cp -a ${UBUNTU_DIR}/debian ${UBUNTU_DIR}/hyperstart-${VERSION}

# run dh_make
cd ${UBUNTU_DIR}/hyperstart-${VERSION}
dh_make -s -y -f ../hyperstart-${VERSION}.tar.gz

# run dpkg-buildpackage
dpkg-buildpackage -us -uc -rfakeroot

#clean up intermediate files
rm -rf ${UBUNTU_DIR}/hyperstart-${VERSION}

