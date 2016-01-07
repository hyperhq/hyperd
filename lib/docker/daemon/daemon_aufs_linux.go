// +build !exclude_graphdriver_aufs

package daemon

import (
	"github.com/golang/glog"
	"github.com/hyperhq/hyper/lib/docker/daemon/graphdriver"
	"github.com/hyperhq/hyper/lib/docker/daemon/graphdriver/aufs"
)

// Given the graphdriver ad, if it is aufs, then migrate it.
// If aufs driver is not built, this func is a noop.
func migrateIfAufs(driver graphdriver.Driver, root string) error {
	if ad, ok := driver.(*aufs.Driver); ok {
		glog.V(2).Infof("Migrating existing containers")
		if err := ad.Migrate(root, setupInitLayer); err != nil {
			return err
		}
	}
	return nil
}
