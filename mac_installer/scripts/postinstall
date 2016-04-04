#!/bin/bash

set -e

INSTALL_BASE=/opt/hyper
INSTALL_IMAGE=${INSTALL_BASE}/static/images
INSTALL_CONFIG=${INSTALL_BASE}/static/config

TARGET_BASE=/var/lib/hyper
TARGET_GRAPH=${TARGET_BASE}/graph
TARGET_VBOX=${TARGET_BASE}/vbox
TARGET_IMAGE=${TARGET_VBOX}/images
TARGET_LAYERS=${TARGET_VBOX}/layers

HYPER_SOCK=/var/run/hyper.sock

PULLER_ID=1000000000000000000000000000000000000000000000000000000000000000

mkdir -p ${TARGET_BASE}
mkdir -p ${TARGET_GRAPH}
mkdir -p ${TARGET_VBOX}
mkdir -p ${TARGET_IMAGE}
mkdir -p ${TARGET_LAYERS}

# prepare base image
if [ -e ${TARGET_IMAGE}/${PULLER_ID}.vdi ]; then 
	rm -f ${TARGET_IMAGE}/${PULLER_ID}.vdi
fi
if [ -e ${TARGET_IMAGE}/base.vdi ]; then 
	rm -f ${TARGET_IMAGE}/base.vdi
fi
ln -s ${INSTALL_IMAGE}/${PULLER_ID}.vdi ${TARGET_IMAGE}/
ln -s ${INSTALL_IMAGE}/base.vdi ${TARGET_IMAGE}/

# prepare repo config
if [ ! -e ${TARGET_BASE}/repositories-vbox ]; then
	cp -r ${INSTALL_CONFIG}/repositories-vbox ${TARGET_BASE}/repositories-vbox
fi

if [ -e ${TARGET_GRAPH}/${PULLER_ID} ] ; then 
	rm -rf ${TARGET_GRAPH}/${PULLER_ID}
fi
cp -r ${INSTALL_CONFIG}/${PULLER_ID} ${TARGET_GRAPH}/

if [ -e ${TARGET_LAYERS}/${PULLER_ID} ] ; then
	rm -f ${TARGET_LAYERS}/${PULLER_ID}
fi
cp ${INSTALL_CONFIG}/layer ${TARGET_LAYERS}/${PULLER_ID}

# load service
if [ -e /Library/LaunchDaemons/sh.hyper.hyper.plist ] ; then
	rm -f /Library/LaunchDaemons/sh.hyper.hyper.plist
fi
cp ${INSTALL_CONFIG}/sh.hyper.hyper.plist /Library/LaunchDaemons/sh.hyper.hyper.plist
/bin/launchctl load "/Library/LaunchDaemons/sh.hyper.hyper.plist"

# ln binary to bin
mkdir -p /usr/local/bin
if [ -e /usr/local/bin/hyperctl ] ; then
	rm -f /usr/local/bin/hyperctl
fi
ln -s /opt/hyper/bin/hyperctl /usr/local/bin/hyperctl


