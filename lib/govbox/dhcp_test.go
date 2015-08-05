package virtualbox

import "testing"

func TestDHCPs(t *testing.T) {
	m, err := DHCPs()
	if err != nil {
		t.Fatal(err)
	}

	for _, dhcp := range m {
		t.Logf("%+v", dhcp)
	}
}
