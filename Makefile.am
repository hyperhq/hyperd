export GOPATH:=$(abs_top_srcdir)/Godeps/_workspace:$(GOPATH)
if WITH_XEN
HYPER_BULD_TAGS=with_xen libdm_no_deferred_remove
else
HYPER_BULD_TAGS=libdm_no_deferred_remove
endif

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
