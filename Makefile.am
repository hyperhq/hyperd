override GOPATH:=$(abs_top_srcdir:/src/github.com/hyperhq/hyper=):$(abs_top_srcdir)/Godeps/_workspace:$(GOPATH)
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
install-exec-local: 
	$(INSTALL_PROGRAM) hyper $(bindir)
	$(INSTALL_PROGRAM) hyperd $(bindir)

build-hyperd:
	go build -tags "static_build $(HYPER_BULD_TAGS)" $(VERSION_PARAM) hyperd.go
build-hyper:
	go build $(VERSION_PARAM) hyper.go
