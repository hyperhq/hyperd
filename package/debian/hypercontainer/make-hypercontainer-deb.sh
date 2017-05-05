#!/bin/bash

PROJECT=$(readlink -f $(dirname $0)/../../..)
DEBIAN_DIR=${PROJECT}/package/debian/hypercontainer
VERSION=${VERSION:-0.8.1}
BRANCH=${BRANCH:-master}

if [ $# -gt 0 ] ; then
    VERSION=$1
fi

# install addtional pkgs in order to build deb pkg
#sudo apt-get install -y autoconf automake pkg-config dh-make libdevmapper-dev libsqlite3-dev libvirt-dev -qq

# get hyperd tar ball
cd $PROJECT
git archive --format=tar.gz ${BRANCH} > ${DEBIAN_DIR}/hypercontainer-${VERSION}.tar.gz

# prepair to create source pkg
mkdir -p ${DEBIAN_DIR}/hypercontainer-${VERSION}
cd ${DEBIAN_DIR}
tar -zxf hypercontainer-${VERSION}.tar.gz -C ${DEBIAN_DIR}/hypercontainer-${VERSION}

# in order to use debian/* to create deb, so put them in the hypercontainer.
cp -a ${DEBIAN_DIR}/debian ${DEBIAN_DIR}/hypercontainer-${VERSION}

# run dh_make
cd ${DEBIAN_DIR}/hypercontainer-${VERSION}
export DEBFULLNAME=Hyper Dev Team
dh_make -s -y -f ../hypercontainer-${VERSION}.tar.gz -e dev@hyper.sh

# run dpkg-buildpackage
dpkg-buildpackage -us -uc -b -rfakeroot

#clean up intermediate files
rm -rf ${DEBIAN_DIR}/hypercontainer-${VERSION}

