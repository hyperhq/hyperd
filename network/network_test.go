package network

import (
	"testing"
)

func TestInitNetwork(t *testing.T) {
	if err := InitNetwork("hyper-test", "192.168.138.1/24"); err != nil {
		t.Error("create hyper-test bridge failed")
	}

	if err := DeleteBridge("hyper-test"); err != nil {
		t.Error("delete hyper-test bridge failed")
	}

	t.Log("bridge check finished.")
}

func TestAllocate(t *testing.T) {
	if err := InitNetwork("hyper-test", "192.168.138.1/24"); err != nil {
		t.Error("create hyper-test bridge failed")
	}

	if setting, err := Allocate("192.168.138.2"); err != nil {
		t.Error("allocate tap device and ip failed")
	} else {
		t.Log("alocate tap device finished. bridge %s, device %s, ip %s, gateway %s",
		      setting.Bridge, setting.Device, setting.IPAddress, setting.Gateway)

		if err := Release("192.168.138.2", setting.File); err != nil {
			t.Error("release ip failed")
		}
	}

	if err := DeleteBridge("hyper-test"); err != nil {
		t.Error("delete hyper-test bridge failed")
	}

	t.Log("allocate finished")
}
