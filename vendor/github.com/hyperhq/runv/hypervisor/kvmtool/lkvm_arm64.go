// +build linux,arm64

package kvmtool

func arch_arguments() []string {
	return []string{"--irqchip", "gicv3"}
}
