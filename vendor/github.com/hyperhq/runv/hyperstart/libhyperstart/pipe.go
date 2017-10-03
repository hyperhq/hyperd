package libhyperstart

import "io"

type inPipe struct {
	h    Hyperstart
	c, p string
}

func (p *inPipe) Write(data []byte) (n int, err error) {
	return p.h.WriteStdin(p.c, p.p, data)
}

func (p *inPipe) Close() error {
	return p.h.CloseStdin(p.c, p.p)
}

type outPipe struct {
	h    Hyperstart
	c, p string
}

func (p *outPipe) Read(data []byte) (n int, err error) {
	return p.h.ReadStdout(p.c, p.p, data)
}

type errPipe struct {
	h    Hyperstart
	c, p string
}

func (p *errPipe) Read(data []byte) (n int, err error) {
	return p.h.ReadStderr(p.c, p.p, data)
}

func StdioPipe(h Hyperstart, c, p string) (io.WriteCloser, io.Reader, io.Reader) {
	return &inPipe{h: h, c: c, p: p}, &outPipe{h: h, c: c, p: p}, &errPipe{h: h, c: c, p: p}
}
