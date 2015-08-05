package virtualbox

import (
	"testing"
)

func init() {
	Verbose = true
}

func TestVBMOut(t *testing.T) {
	b, err := vbmOut("list", "vms")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s", b)
}
