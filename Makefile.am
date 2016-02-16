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

VERSION_PARAM=-ldflags "-X github.com/hyperhq/hyper/utils.VERSION $(VERSION)"

all-local: build-hyperd build-hyper
clean-local:
	-rm -f hyperd hyper
	-rm -f Godeps/_workspace/src/github.com/opencontainers/specs/config-linux.go Godeps/_workspace/src/github.com/opencontainers/specs/runtime-config-linux.go
install-exec-local: 
	$(INSTALL_PROGRAM) hyper $(bindir)
	$(INSTALL_PROGRAM) hyperd $(bindir)

# supporting linux container on non-linux platform (copy for catering to go build)
if ON_LINUX
linux_container: Godeps/_workspace/src/github.com/opencontainers/specs/config_linux.go Godeps/_workspace/src/github.com/opencontainers/specs/runtime_config_linux.go
else
linux_container: Godeps/_workspace/src/github.com/opencontainers/specs/config-linux.go Godeps/_workspace/src/github.com/opencontainers/specs/runtime-config-linux.go
Godeps/_workspace/src/github.com/opencontainers/specs/config-linux.go:
	-cp Godeps/_workspace/src/github.com/opencontainers/specs/config_linux.go Godeps/_workspace/src/github.com/opencontainers/specs/config-linux.go
Godeps/_workspace/src/github.com/opencontainers/specs/runtime-config-linux.go:
	-cp Godeps/_workspace/src/github.com/opencontainers/specs/runtime_config_linux.go Godeps/_workspace/src/github.com/opencontainers/specs/runtime-config-linux.go
endif

build-hyperd: linux_container
	go build -tags "static_build $(HYPER_BULD_TAGS)" $(VERSION_PARAM) hyperd.go
build-hyper:
	go build $(VERSION_PARAM) hyper.go
