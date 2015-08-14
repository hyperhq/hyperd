package builder

import (
	"fmt"

	"github.com/hyperhq/hyper/daemon"
	"github.com/hyperhq/hyper/utils"
)

func GetDaemon() (*daemon.Daemon, error) {
	d := utils.HYPER_DAEMON
	if d == nil {
		return nil, fmt.Errorf("Can not find hyper daemon")
	}

	return d.(*daemon.Daemon), nil
}
