package graphdriver

import (
	"path/filepath"
	"syscall"

	"github.com/hyperhq/runv/lib/glog"
)

type DiffDiskDriver interface {
	Driver
	CopyDiff(id, sourceId string) error
}

const (
	FsMagicAufs     = FsMagic(0x61756673)
	FsMagicBtrfs    = FsMagic(0x9123683E)
	FsMagicCramfs   = FsMagic(0x28cd3d45)
	FsMagicExtfs    = FsMagic(0x0000EF53)
	FsMagicF2fs     = FsMagic(0xF2F52010)
	FsMagicJffs2Fs  = FsMagic(0x000072b6)
	FsMagicJfs      = FsMagic(0x3153464a)
	FsMagicNfsFs    = FsMagic(0x00006969)
	FsMagicRamFs    = FsMagic(0x858458f6)
	FsMagicReiserFs = FsMagic(0x52654973)
	FsMagicSmbFs    = FsMagic(0x0000517B)
	FsMagicSquashFs = FsMagic(0x73717368)
	FsMagicTmpFs    = FsMagic(0x01021994)
	FsMagicXfs      = FsMagic(0x58465342)
	FsMagicZfs      = FsMagic(0x2fc12fc1)
	FsMagicVfs      = FsMagic(0x00000001)
	FsMagicHfs      = FsMagic(0x00004244)
	FsMagicHfsplus  = FsMagic(0x00000011)
	FsMagicHpfs     = FsMagic(0xF995E849)
)

var (
	FsNames = map[FsMagic]string{
		FsMagicAufs:        "aufs",
		FsMagicBtrfs:       "btrfs",
		FsMagicCramfs:      "cramfs",
		FsMagicExtfs:       "extfs",
		FsMagicF2fs:        "f2fs",
		FsMagicJffs2Fs:     "jffs2",
		FsMagicJfs:         "jfs",
		FsMagicNfsFs:       "nfs",
		FsMagicRamFs:       "ramfs",
		FsMagicReiserFs:    "reiserfs",
		FsMagicSmbFs:       "smb",
		FsMagicSquashFs:    "squashfs",
		FsMagicTmpFs:       "tmpfs",
		FsMagicXfs:         "xfs",
		FsMagicZfs:         "zfs",
		FsMagicHfs:         "hfs",
		FsMagicHfsplus:     "hfs+",
		FsMagicHpfs:        "hpfs",
		FsMagicUnsupported: "unsupported",
	}

	// Slice of drivers that should be used in an order
	Priority = []string{
		"aufs",
		"overlay",
		"devicemapper",
		"vbox",
		"vfs",
	}
)

func GetFSMagic(rootpath string) (FsMagic, error) {
	var buf syscall.Statfs_t
	fd, err := syscall.Open(filepath.Dir(rootpath), 0, 0700)
	if err != nil {
		return 0, err
	}
	if err := syscall.Fstatfs(fd, &buf); err != nil {
		return 0, err
	}
	glog.Errorf("%x\n", buf.Type)
	return FsMagic(buf.Type), nil
}
