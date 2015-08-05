// +build !exclude_graphdriver_devicemapper

package daemon

import (
	_ "github.com/hyperhq/hyper/lib/docker/daemon/graphdriver/devmapper"
)
