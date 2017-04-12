#!/bin/bash
#
# ------------ update vendor/ with govendor -----------
#
# Usage:
#
#     # update runv
#     > hack/update-govendor.sh github.com/hyperhq/runv/...
#
#     # update all available vendors
#     > hack/update-govendor.sh
#
#     # dry run
#     > hack/update-govendor.sh -n
#


which govendor > /dev/null 2>&1 
ret=$?
if [ $ret -ne 0 ] ; then
	echo "govendor does not exist, install it firstly..."

	if [ "${GOPATH}x" == "x" ] ; then
		echo "GOPATH env is empty, can not install govendor" 1>&2
		exit 1
	fi

	if go get -u github.com/kardianos/govendor ; then
		export PATH=${GOPATH}/bin:${PATH}
		echo "govendor installed."
	else
		echo "govendor install failed" 1>&2
		exit 1
	fi
fi

if [ $# -gt 0 ] ; then
	govendor update $@
	ret=$?
else
	govendor update +vendor
	ret=$?
fi

if [ $ret -eq 0 ] ; then
	echo "govendor updated [$@]."
else
	echo "govendor update failed [$@]." 1>&2
	exit 1
fi

