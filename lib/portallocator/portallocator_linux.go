package portallocator

import (
	"bufio"
	"fmt"
	"os"
)

func init() {
	const portRangeKernelParam = "/proc/sys/net/ipv4/ip_local_port_range"
	portRangeFallback := fmt.Sprintf("using fallback port range %d-%d", beginPortRange, endPortRange)

	file, err := os.Open(portRangeKernelParam)
	if err != nil {
		fmt.Printf("port allocator - %s due to error: %v", portRangeFallback, err)
		return
	}
	var start, end int
	n, err := fmt.Fscanf(bufio.NewReader(file), "%d\t%d", &start, &end)
	if n != 2 || err != nil {
		if err == nil {
			err = fmt.Errorf("unexpected count of parsed numbers (%d)", n)
		}
		fmt.Printf("port allocator - failed to parse system ephemeral port range from %s - %s: %v", portRangeKernelParam, portRangeFallback, err)
		return
	}
	beginPortRange = start
	endPortRange = end
}
