// +build exclude_graphdriver_aufs

package daemon

import (
	"github.com/hyperhq/hyper/lib/docker/daemon/graphdriver"
)

func migrateIfAufs(driver graphdriver.Driver, root string) error {
	return nil
}
