package virtualbox

import "testing"

func TestHostonlyNets(t *testing.T) {
	m, err := HostonlyNets()
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range m {
		t.Logf("%+v", n)
	}
}
