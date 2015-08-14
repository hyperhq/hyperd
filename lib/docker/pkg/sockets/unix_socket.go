// +build linux

package sockets

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"syscall"

	"github.com/docker/libcontainer/user"
	"github.com/hyperhq/hyper/lib/docker/pkg/listenbuffer"
	"github.com/hyperhq/runv/lib/glog"
)

func NewUnixSocket(path, group string, activate <-chan struct{}) (net.Listener, error) {
	if err := syscall.Unlink(path); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	mask := syscall.Umask(0777)
	defer syscall.Umask(mask)
	l, err := listenbuffer.NewListenBuffer("unix", path, activate)
	if err != nil {
		return nil, err
	}
	if err := setSocketGroup(path, group); err != nil {
		l.Close()
		return nil, err
	}
	if err := os.Chmod(path, 0660); err != nil {
		l.Close()
		return nil, err
	}
	return l, nil
}

func setSocketGroup(path, group string) error {
	if group == "" {
		return nil
	}
	if err := changeGroup(path, group); err != nil {
		if group != "docker" {
			return err
		}
		glog.V(1).Infof("Warning: could not change group %s to docker: %v", path, err)
	}
	return nil
}

func changeGroup(path string, nameOrGid string) error {
	gid, err := lookupGidByName(nameOrGid)
	if err != nil {
		return err
	}
	glog.V(1).Infof("%s group found. gid: %d", nameOrGid, gid)
	return os.Chown(path, 0, gid)
}

func lookupGidByName(nameOrGid string) (int, error) {
	groupFile, err := user.GetGroupPath()
	if err != nil {
		return -1, err
	}
	groups, err := user.ParseGroupFileFilter(groupFile, func(g user.Group) bool {
		return g.Name == nameOrGid || strconv.Itoa(g.Gid) == nameOrGid
	})
	if err != nil {
		return -1, err
	}
	if groups != nil && len(groups) > 0 {
		return groups[0].Gid, nil
	}
	gid, err := strconv.Atoi(nameOrGid)
	if err == nil {
		glog.Warningf("Could not find GID %d", gid)
		return gid, nil
	}
	return -1, fmt.Errorf("Group %s not found", nameOrGid)
}
