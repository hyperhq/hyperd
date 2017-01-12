package virtualbox

import (
	"fmt"
	"io"
	"os/exec"
)

// Convert the raw format device to vmdk format
func ConvertRawToImage(filename, dest, format string) error {
	if format != "VDI" && format != "VMDK" && format != "VHD" {
		return fmt.Errorf("Unsupported image format!")
	}
	args := []string{"convertfromraw", filename, dest, "--format", format}
	return vbm(args...)
}

// MakeDiskImage makes a disk image at dest with the given size in MB. If r is
// not nil, it will be read as a raw disk image to convert from.
func MakeDiskImage(dest string, size uint, r io.Reader) error {
	// Convert a raw image from stdin to the dest VMDK image.
	sizeBytes := int64(size) << 20 // usually won't fit in 32-bit int (max 2GB)
	cmd := exec.Command(VBM, "convertfromraw", "stdin", dest,
		fmt.Sprintf("%d", sizeBytes), "--format", "VMDK")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	n, err := io.Copy(stdin, r)
	if err != nil {
		return err
	}

	// The total number of bytes written to stdin must match sizeBytes, or
	// VBoxManage.exe on Windows will fail. Fill remaining with zeros.
	if left := sizeBytes - n; left > 0 {
		if err := ZeroFill(stdin, left); err != nil {
			return err
		}
	}

	// cmd won't exit until the stdin is closed.
	if err := stdin.Close(); err != nil {
		return err
	}

	return cmd.Wait()
}

// ZeroFill writes n zero bytes into w.
func ZeroFill(w io.Writer, n int64) error {
	const blocksize = 32 << 10
	zeros := make([]byte, blocksize)
	var k int
	var err error
	for n > 0 {
		if n > blocksize {
			k, err = w.Write(zeros)
		} else {
			k, err = w.Write(zeros[:n])
		}
		if err != nil {
			return err
		}
		n -= int64(k)
	}
	return nil
}
