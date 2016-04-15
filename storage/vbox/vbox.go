package vbox

import (
	"fmt"

	"github.com/hyperhq/hyperd/utils"
)

// For device mapper, we do not need to mount the container to sharedDir.
// All of we need to provide the block device name of container.
func MountContainerToSharedDir(containerId, sharedDir, devPrefix string) (string, error) {
	devFullName := fmt.Sprintf("%s/vbox/images/%s.vdi", utils.HYPER_ROOT, containerId)
	return devFullName, nil
}

func AttachFiles(containerId, devPrefix, fromFile, toDir, rootPath, perm, uid, gid string) error {
	return nil
}
