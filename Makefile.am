SUBDIRS=hypervisor
if WITH_XEN
cgo_ldflags=CGO_LDFLAGS="-Lhypervisor/xen -lxenlight -lxenctrl -lhyperxl"
else
cgo_ldflags=CGO_LDFLAGS="-Lhypervisor/xen-lhyperxl"
endif

all-local: build-hyperd build-hyper
clean-local:
	-rm -f hyperd hyper
install-exec-local: 
	$(INSTALL_PROGRAM) hyper $(bindir)
	$(INSTALL_PROGRAM) hyperd $(bindir)

build-hyperd:
	$(cgo_ldflags) godep go build hyperd.go
build-hyper:
	godep go build hyper.go
