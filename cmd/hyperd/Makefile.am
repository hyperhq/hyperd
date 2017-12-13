if WITH_XEN
XEN_BUILD_TAG=with_xen
else
XEN_BUILD_TAG=
endif

if WITH_XEN490
XEN490_BUILD_TAG=with_xen490
else
XEN490_BUILD_TAG=
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

HYPER_BULD_TAGS=$(XEN_BUILD_TAG) $(XEN490_BUILD_TAG) $(LIBVIRT_BUILD_TAG) $(BTRFS_BUILD_TAG) libdm_no_deferred_remove

all-local: build-hyperd
clean-local:
	-rm -f hyperd 
install-exec-local:
	$(INSTALL_PROGRAM) hyperd $(bindir)

hyperd-local: build-hyperd
build-hyperd:
	go build -tags "static_build $(HYPER_BULD_TAGS)" $(VERSION_PARAM) hyperd.go
