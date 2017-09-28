export GO15VENDOREXPERIMENT=1
if WITH_XEN
XEN_BUILD_TAG=with_xen
else
XEN_BUILD_TAG=
endif

if WITH_LIBVIRT
LIBVIRT_BUILD_TAG=with_libvirt
else
LIBVIRT_BUILD_TAG=
endif

if WITH_BTRFS
if WITH_BTRFS_HEADER_VERSION
BTRFS_BUILD_TAG=
else
BTRFS_BUILD_TAG=btrfs_noversion
endif
else
BTRFS_BUILD_TAG=exclude_graphdriver_btrfs
endif

HYPER_BULD_TAGS=$(XEN_BUILD_TAG) $(LIBVIRT_BUILD_TAG) $(BTRFS_BUILD_TAG) libdm_no_deferred_remove
if ON_DARWIN
SUBDIRS=mac_installer
endif

VERSION_PARAM=-ldflags "-X github.com/hyperhq/hyperd/utils.VERSION=$(VERSION) -X github.com/hyperhq/hyperd/utils.GITCOMMIT=`git describe --dirty --always --tags 2> /dev/null || true`"

all-local: build-hyperd build-hyperctl build-vmlogd
clean-local:
	-rm -f hyperd hyperctl vmlogd
install-exec-local:
	$(INSTALL_PROGRAM) hyperctl $(bindir)
	$(INSTALL_PROGRAM) hyperd $(bindir)
	$(INSTALL_PROGRAM) vmlogd $(bindir)

build-hyperd:
	go build -tags "static_build $(HYPER_BULD_TAGS)" $(VERSION_PARAM) hyperd.go
build-hyperctl:
	go build $(VERSION_PARAM) hyperctl.go
build-vmlogd:
	go build $(VERSION_PARAM) vmlogd.go
