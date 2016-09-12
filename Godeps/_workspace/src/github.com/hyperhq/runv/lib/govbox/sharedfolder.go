package virtualbox

type SharedFolder struct {
	Name      string
	Path      string
	Automount bool
	Transient bool
	Readonly  bool
}
