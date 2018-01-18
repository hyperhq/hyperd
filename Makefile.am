export GO15VENDOREXPERIMENT=1
export VERSION_PARAM=-ldflags "-X github.com/hyperhq/hyperd/utils.VERSION=$(VERSION) -X github.com/hyperhq/hyperd/utils.GITCOMMIT=`git describe --dirty --always --tags 2> /dev/null || true`"
if ON_LINUX
export GO_GCFLAGS=`if [ "$(GODEBUG)" != "" ]; then echo "-N -l"; else echo ""; fi`
endif

SUBDIRS=cmd

