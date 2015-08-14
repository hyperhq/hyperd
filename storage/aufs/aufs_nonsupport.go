// +build !linux

package aufs

func MountContainerToSharedDir(containerId, rootDir, sharedDir, mountLabel string) (string, error) {
	return "", nil
}

func AttachFiles(containerId, fromFile, toDir, rootDir, perm, uid, gid string) error {
	return nil
}
