#!/bin/bash

PROJECT=$(readlink -f $(dirname $0)/../../..)
UBUNTU_DIR=${PROJECT}/package/ubuntu/hypercontainer
VERSION=0.6.2

if [ $# -gt 0 ] ; then
    VERSION=$1
fi

# install addtional pkgs in order to build deb pkg
sudo apt-get install -y autoconf automake pkg-config libdevmapper-dev libsqlite3-dev libvirt-dev libxen-dev uuid-dev golang xen-hypervisor-4.6-amd64 -qq

# get hyperd tar ball
cd $PROJECT
git archive --format=tar.gz master > ${UBUNTU_DIR}/hypercontainer-${VERSION}.tar.gz

# prepair to create source pkg
mkdir -p ${UBUNTU_DIR}/hypercontainer-${VERSION}
cd ${UBUNTU_DIR}
tar -zxvf hypercontainer-${VERSION}.tar.gz -C ${UBUNTU_DIR}/hypercontainer-${VERSION}

# in order to use debian/* to create deb, so put them in the hypercontainer.
cp -a ${UBUNTU_DIR}/debian ${UBUNTU_DIR}/hypercontainer-${VERSION}

# run dh_make
cd ${UBUNTU_DIR}/hypercontainer-${VERSION}
dh_make -s -y -f ../hypercontainer-${VERSION}.tar.gz

# run dpkg-buildpackage
dpkg-buildpackage -us -uc -rfakeroot

#clean up intermediate files
rm -rf ${UBUNTU_DIR}/hypercontainer-${VERSION}

