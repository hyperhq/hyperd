export GOPATH:=$(abs_top_srcdir)/Godeps/_workspace:$(GOPATH)
if WITH_XEN
HYPER_BULD_TAGS=with_xen
else
HYPER_BULD_TAGS=
endif

all-local: build-hyperd build-hyper
clean-local:
	-rm -f hyperd hyper
install-exec-local: 
	$(INSTALL_PROGRAM) hyper $(bindir)
	$(INSTALL_PROGRAM) hyperd $(bindir)

build-hyperd:
	godep go build -tags "static_build $(HYPER_BULD_TAGS)" hyperd.go
build-hyper:
	godep go build hyper.go
