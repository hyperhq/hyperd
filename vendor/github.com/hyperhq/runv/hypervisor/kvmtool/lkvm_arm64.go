// +build linux,arm64

package kvmtool

import (
	"bufio"
	"os"
	"strings"
)

// On ARM platform, we have different gic interrupt controllers.
// We have to detect the correct gic chip to set parameter for lkvm.
func getGicInfo() (info string) {
	gicinfo, err := os.Open("/proc/interrupts")
	if err != nil {
		return "unknown"
	}

	defer gicinfo.Close()

	scanner := bufio.NewScanner(gicinfo)
	for scanner.Scan() {
		newline := scanner.Text()
		list := strings.Fields(newline)

		for _, item := range list {
			if strings.EqualFold(item, "GICv2") {
				return "gicv2"
			} else if strings.EqualFold(item, "GICv3") ||
				strings.EqualFold(item, "GICv4") {
				return "gicv3"
			}
		}
	}

	return "unknown"
}

func arch_arguments() []string {
	return []string{"--irqchip", getGicInfo()}
}
