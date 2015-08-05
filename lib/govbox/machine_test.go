package virtualbox

import "testing"

func TestMachine(t *testing.T) {
	ms, err := ListMachines()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ms {
		t.Logf("%+v", m)
	}
}
