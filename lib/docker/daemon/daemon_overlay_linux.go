// +build !exclude_graphdriver_overlay

package daemon

import (
	_ "github.com/hyperhq/hyper/lib/docker/daemon/graphdriver/overlay"
)
