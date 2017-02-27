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
