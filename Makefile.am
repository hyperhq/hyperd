export GOPATH:=$(abs_top_srcdir)/Godeps/_workspace:$(GOPATH)
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

HYPER_BULD_TAGS=$(XEN_BUILD_TAG) $(LIBVIRT_BUILD_TAG) libdm_no_deferred_remove exclude_graphdriver_btrfs
if ON_DARWIN
SUBDIRS=mac_installer
endif

VERSION_PARAM=-ldflags "-X github.com/hyperhq/hyperd/utils.VERSION $(VERSION)"

all-local: build-hyperd build-hyperctl
clean-local:
	-rm -f hyperd hyperctl
install-exec-local: 
	$(INSTALL_PROGRAM) hyperctl $(bindir)
	$(INSTALL_PROGRAM) hyperd $(bindir)

build-hyperd:
	go build -tags "static_build $(HYPER_BULD_TAGS)" $(VERSION_PARAM) hyperd.go
build-hyperctl:
	go build $(VERSION_PARAM) hyperctl.go
