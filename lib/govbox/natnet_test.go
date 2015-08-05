package virtualbox

import "testing"

func TestNATNets(t *testing.T) {
	m, err := NATNets()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%+v", m)
}
