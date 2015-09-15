#!/bin/bash

HYPER_SOCK=/var/run/hyper.sock
PATH=/usr/local/bin:/usr/bin:/bin

if /bin/launchctl list "sh.hyper.hyper" &> /dev/null; then
	/bin/launchctl unload "/Library/LaunchDaemons/sh.hyper.hyper.plist"
fi

# clean socket
if [ -e ${HYPER_SOCK} ] ; then
	rm -f ${HYPER_SOCK}
fi

# clean puller vm
if which vboxmanage &> /dev/null ; then
	if vboxmanage showvminfo hyper-mac-pull-vm &> /dev/null; then
		vboxmanage controlvm hyper-mac-pull-vm poweroff || true
		vboxmanage unregistervm hyper-mac-pull-vm --delete || true
	fi
fi

rm -rf /var/run/hyper/vm/hyper-mac-pull-vm
rm -rf /var/run/hyper/hyper-mac-pull-vm

