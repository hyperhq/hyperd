#!/bin/bash

PATH=/usr/local/bin:/usr/bin:/bin

HYPER_HOME=/opt/hyper
HYPER_SOCK=/var/run/hyper.sock
HYPER_RUNTIME=/var/lib/hyper
HYPER_VM=/var/run/hyper
HYPER_VM_TMP=${HYPER_VM}/vm
HYPER_LOG=/var/log/hyper

UNLINK="${HYPER_HOME}/bin/hunlink"
RMDIR="rmdir"
RM="rm"

PURGE="FALSE"
if [ $# -gt 0 -a "$1x" == "--purgex" ] ; then
	PURGE="TRUE"
fi

# Stop Service and Remove Service Configuration
if /bin/launchctl list "sh.hyper.hyper" &> /dev/null; then
	/bin/launchctl unload "/Library/LaunchDaemons/sh.hyper.hyper.plist"
fi
${RM} -f /Library/LaunchDaemons/sh.hyper.hyper.plist

# clean puller vm
if which vboxmanage &> /dev/null ; then
	if vboxmanage showvminfo hyper-mac-pull-vm &> /dev/null; then
		vboxmanage controlvm hyper-mac-pull-vm poweroff || true
		vboxmanage unregistervm hyper-mac-pull-vm --delete || true
	fi
fi
rm -rf ${HYPER_VM_TMP}/hyper-mac-pull-vm
rm -rf ${HYPER_VM}/hyper-mac-pull-vm

if [ "x$PURGE" != "xFALSE" ] ; then

	if [ -d ${HYPER_VM_TMP} ]; then
		rm -rf ${HYPER_VM_TMP}
	fi

	# Remove the hyper vm dir, include volumes. In normal case, these dir
	# should be empty. But remove carefully in case some were not properly
	# unlinked
	if [ -d ${HYPER_VM} ]; then
		for vmroot in ${HYPER_VM}/* ; do 
			if [ -d ${vmroot}/share_dir ] ; then
				for sharedir in ${vmroot}/share_dir/* ; do
					${UNLINK} ${sharedir}
				done
				${RMDIR} ${vmroot}/share_dir
			fi
			${RM} -f ${vmroot}/*.sock
			${RMDIR} ${vmroot}
		done
		${RMDIR} ${HYPER_VM}
	fi
	
	# Remove the images and vm/container
	${RM} -rf ${HYPER_RUNTIME}/{containers,graph,linkgraph.db,repositories-vbox,run,tmp,trust,vbox,lib,vm,hyper.db}
	if [ -d ${HYPER_RUNTIME} ]; then
		${RMDIR} ${HYPER_RUNTIME}
	fi
fi

# Remove socket, logs, and binaries
${RM} -f  ${HYPER_SOCK}
${RM} -rf ${HYPER_LOG}
${RM} -rf ${HYPER_HOME}

