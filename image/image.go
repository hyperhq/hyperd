package image

import (
	"io"
)

// Exporter provides interface for exporting and importing images
type Exporter interface {
	Load(io.ReadCloser, string, map[string]string, io.Writer) error
	// TODO: Load(net.Context, io.ReadCloser, <- chan StatusMessage) error
	Save([]string, string, map[string]string, io.Writer) error
}
