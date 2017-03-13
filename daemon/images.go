package daemon

import (
	"io"

	"github.com/hyperhq/hyperd/image/tarexport"
)

// ExportImage exports a list of images to the given output stream. The
// exported images are archived into a tar when written to the output
// stream. All images with the given tag and all versions containing
// the same tag are exported. names is the set of tags to export, and
// outStream is the writer which the images are written to.
func (daemon *Daemon) ExportImage(names []string, format string, refs map[string]string, outStream io.Writer) error {
	imageExporter := tarexport.NewTarExporter(daemon.ImageStore(), daemon.LayerStore(), daemon.ReferenceStore())
	return imageExporter.Save(names, format, refs, outStream)
}

// LoadImage uploads a set of images into the repository. This is the
// complement of ImageExport.  The input stream is an uncompressed tar
// ball containing images and metadata.
func (daemon *Daemon) LoadImage(inTar io.ReadCloser, name string, refs map[string]string, outStream io.Writer) error {
	imageExporter := tarexport.NewTarExporter(daemon.ImageStore(), daemon.LayerStore(), daemon.ReferenceStore())
	return imageExporter.Load(inTar, name, refs, outStream)
}
