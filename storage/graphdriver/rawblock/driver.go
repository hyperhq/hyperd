package rawblock

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/docker/docker/daemon/graphdriver"
	"github.com/docker/docker/pkg/idtools"
	"github.com/golang/glog"
	"github.com/opencontainers/runc/libcontainer/label"
	//"github.com/docker/docker/pkg/mount"
)

func init() {
	graphdriver.Register("rawblock", Init)
}

// Driver holds information about the driver, home directory of the driver.
// Driver implements graphdriver.ProtoDriver. It uses only basic vfs operations.
// In order to support layering, the block is created via reflink with the the parent layer
// Driver must be wrapped in NaiveDiffDriver to be used as a graphdriver.Driver
type Driver struct {
	home      string
	backingFs string // host filesystem of the storage
	cow       bool
	blockFs   string // filesystem inside the block
	blockSize uint64 // block size in GB
	uid       int
	gid       int

	sync.Mutex // Protects concurrent modification to active
	active     map[string]int
}

// test if the filesystem on @home supports reflink, return false also if unsure.
func testReflinkSupport(home string) bool {
	file1 := filepath.Join(home, "test-reflink-support-src")
	file2 := filepath.Join(home, "test-reflink-support-dest")
	if _, err := exec.Command("truncate", fmt.Sprintf("--size=%d", 16*1024), file1).CombinedOutput(); err != nil {
		return false
	}
	defer os.RemoveAll(file1)
	defer os.RemoveAll(file2)
	if _, err := exec.Command("cp", "-a", "--reflink=always", file1, file2).CombinedOutput(); err != nil {
		return false
	}
	return true
}

// Init returns a new Raw Block driver.
// This sets the home directory for the driver and returns NaiveDiffDriver.
func Init(home string, options []string, uidMaps, gidMaps []idtools.IDMap) (graphdriver.Driver, error) {
	backingFs := "<unknown>"
	blockFs := "xfs" // TODO: make it configurable
	cow := true
	supported := "supported"

	fsMagic, err := graphdriver.GetFSMagic(home)
	if err != nil {
		return nil, err
	}
	if fsName, ok := graphdriver.FsNames[fsMagic]; ok {
		backingFs = fsName
	}

	// check if they are running over btrfs or xfs
	switch fsMagic {
	case graphdriver.FsMagicBtrfs: // support
	case graphdriver.FsMagicXfs: // check support
		if testReflinkSupport(home) {
			break
		}
		fallthrough
	default:
		cow = false
		supported = "NOT supported"
	}
	glog.Infof("RawBlock: copy-on-write is %s", supported)

	rootUID, rootGID, err := idtools.GetRootUIDGID(uidMaps, gidMaps)
	if err != nil {
		return nil, err
	}
	if err := idtools.MkdirAllAs(home, 0700, rootUID, rootGID); err != nil {
		return nil, err
	}

	d := &Driver{
		home:      home,
		backingFs: backingFs,
		cow:       cow,
		blockFs:   blockFs,
		blockSize: 10, // TODO: make it configurable
		uid:       rootUID,
		gid:       rootGID,
		active:    map[string]int{},
	}
	return graphdriver.NewNaiveDiffDriver(d, uidMaps, gidMaps), nil
}

func (d *Driver) String() string {
	return "rawblock"
}

// Status is used for implementing the graphdriver.ProtoDriver interface.
func (d *Driver) Status() [][2]string {
	return [][2]string{
		{"Backing Filesystem", d.backingFs},
		{"Support Copy-On-Write", fmt.Sprintf("%v", d.cow)},
		{"Block Filesystem", d.blockFs},
		{"Block Size", fmt.Sprintf("%dGB", d.blockSize)},
	}
}

// GetMetadata is used for implementing the graphdriver.ProtoDriver interface.
func (d *Driver) GetMetadata(id string) (map[string]string, error) {
	return nil, nil
}

// Cleanup is used to implement graphdriver.ProtoDriver. There is no cleanup required for this driver.
func (d *Driver) Cleanup() error {
	return nil
}

// Create prepares the filesystem for the rawblock driver and copies the block from the parent.
func (d *Driver) Create(id, parent, mountLabel string) error {
	if err := idtools.MkdirAllAs(filepath.Dir(d.block(id)), 0700, d.uid, d.gid); err != nil {
		return err
	}
	if parent == "" {
		return CreateBlock(d.block(id), d.blockFs, mountLabel, d.blockSize*1024*1024*1024)
	}
	if out, err := exec.Command("cp", "-a", "--reflink=auto", d.block(parent), d.block(id)).CombinedOutput(); err != nil {
		return fmt.Errorf("Failed to reflink:%v:%s", err, string(out))
	}
	return nil
}

func (d *Driver) block(id string) string {
	return filepath.Join(d.home, "blocks", id)
}

func (d *Driver) mntBase() string {
	return filepath.Join(d.home, "mnt")
}

// Remove deletes the content from the directory for a given id.
func (d *Driver) Remove(id string) error {
	if err := os.RemoveAll(d.block(id)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Get returns the directory for the given id.
func (d *Driver) Get(id, mountLabel string) (string, error) {
	d.Lock()
	defer d.Unlock()
	active := d.active[id]
	if active == 0 {
		if err := GetImage(filepath.Join(d.home, "blocks"), d.mntBase(), id, d.blockFs, mountLabel, d.uid, d.gid); err != nil {
			return "", err
		}
	}
	d.active[id] = active + 1
	return filepath.Join(d.mntBase(), id, "rootfs"), nil
}

func (d *Driver) Put(id string) error {
	d.Lock()
	defer d.Unlock()
	active := d.active[id]
	if active > 1 {
		d.active[id] = active - 1
		return nil
	}
	if err := PutImage(d.mntBase(), id); err != nil {
		return err
	}
	delete(d.active, id)
	return nil
}

// Exists checks to see if the directory exists for the given id.
func (d *Driver) Exists(id string) bool {
	_, err := os.Stat(d.block(id))
	return err == nil
}

func CreateBlock(block, fstype, mountLabel string, size uint64) error {
	//opts := []string{"level:s0"}
	//if _, mountLabel, err := label.InitLabels(opts); err == nil {
	//	label.SetFileLabel(dir, mountLabel)
	//}
	if out, err := exec.Command("truncate", fmt.Sprintf("--size=%d", size), block).CombinedOutput(); err != nil {
		return fmt.Errorf("Failed to create block:%v:%s", err, string(out))
	}
	switch fstype {
	case "xfs":
		if out, err := exec.Command("mkfs.xfs", "-f", block).CombinedOutput(); err != nil {
			os.RemoveAll(block)
			return fmt.Errorf("Failed to mkfs the block:%v:%s", err, string(out))
		}
	case "ext4":
		if out, err := exec.Command("mkfs.ext4", "-F", block).CombinedOutput(); err != nil {
			os.RemoveAll(block)
			return fmt.Errorf("Failed to mkfs the block:%v:%s", err, string(out))
		}
	default:
		os.RemoveAll(block)
		return fmt.Errorf("Unsupported filesystem for the block: %s", fstype)
	}
	return nil
}

func joinMountOptions(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return a + "," + b
}

func mount(block, mnt, fstype string, mountLabel string) error {
	options := "loop"

	if fstype == "xfs" {
		// XFS needs nouuid or it can't mount filesystems with the same fs
		options = joinMountOptions(options, "nouuid")
	}

	options = joinMountOptions(options, label.FormatMountLabel("", mountLabel))

	if out, err := exec.Command("mount", "-t", fstype, "-o", options, block, mnt).CombinedOutput(); err != nil {
		return fmt.Errorf("Failed to mount block:%v:%s", err, string(out))
	}
	return nil
}

func GetImage(blockBase, mntBase, id, fstype, mountLabel string, uid, gid int) error {
	block := filepath.Join(blockBase, id)
	mnt := filepath.Join(mntBase, id)
	rootFs := filepath.Join(mntBase, id, "rootfs")

	if err := idtools.MkdirAllAs(mnt, 0755, uid, gid); err != nil {
		return err
	}

	if err := mount(block, mnt, fstype, mountLabel); err != nil {
		os.RemoveAll(mnt)
		return err
	}

	if err := idtools.MkdirAllAs(rootFs, 0755, uid, gid); err != nil && !os.IsExist(err) {
		exec.Command("umount", "-d", mnt).CombinedOutput()
		os.RemoveAll(mnt)
		return err
	}

	return nil
}

func PutImage(mntBase, id string) error {
	mnt := filepath.Join(mntBase, id)
	if out, err := exec.Command("umount", "-d", mnt).CombinedOutput(); err != nil {
		return fmt.Errorf("Failed to umount block:%v:%s", err, string(out))
	}
	os.RemoveAll(mnt)
	return nil
}
