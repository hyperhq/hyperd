if ON_DARWIN
pkg-local:
	mkdir -p dist/opt/hyper/bin
	mkdir -p dist/opt/hyper/examples
	cp ../hyper ../hyperd dist/opt/hyper/bin/
	cp ../examples/* dist/opt/hyper/examples/
	pkgbuild --identifier sh.hyper.hyper --version $(VERSION) --ownership recommended --root dist --scripts scripts output.pkg
	productbuild --distribution distribution.xml --resources resources --package-path output.pkg --version $(VERSION) ../hyper.pkg
endif
