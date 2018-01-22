all-local: build-vmlogd
clean-local:
	-rm -f vmlogd
install-exec-local:
	$(INSTALL_PROGRAM) vmlogd $(bindir)

vmlogd-local: build-vmlogd
build-vmlogd:
	go build -gcflags="$(GO_GCFLAGS)" $(VERSION_PARAM) vmlogd.go
