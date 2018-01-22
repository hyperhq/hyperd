all-local: build-hyperctl
clean-local:
	-rm -f hyperctl
install-exec-local:
	$(INSTALL_PROGRAM) hyperctl $(bindir)

hyperctl-local: build-hyperctl
build-hyperctl:
	go build -gcflags="$(GO_GCFLAGS)" $(VERSION_PARAM) hyperctl.go
