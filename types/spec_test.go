package types

import (
	"flag"
	"testing"
)

func init() {
	flag.Set("alsologtostderr", "true")
	flag.Set("v", "5")
}

//PortMapping related tests
func TestPortRange(t *testing.T) {
	var (
		res *_PortRange
		err error
	)

	t.Log("> testing single port")
	t.Log("> > testing read empty port range")
	res, err = readPortRange("")
	if err != nil {
		t.Fatal("readPortRange cann't handle default range")
	}
	if res == nil || res.start != 1025 || res.end != 65535 {
		t.Fatalf("readPortRange mistake handles default range, result: %#v", res)
	}

	t.Log("> > testing read none number port")
	res, err = readPortRange("whatever")
	if err == nil {
		t.Fatal("didn't found non number port")
	}
	t.Logf("----successfully found non number port: %v", err)

	t.Log("> > testing out of range port")
	res, err = readPortRange("-1")
	if err == nil {
		t.Fatal("didn't found negative port")
	}
	t.Logf("----successfully found negative number: %v", err)

	res, err = readPortRange("65536")
	if err == nil {
		t.Fatal("didn't found too large port")
	}
	t.Logf("----successfully found too large number: %v", err)

	t.Log("> > testing correct port")
	res, err = readPortRange("22")
	if err != nil {
		t.Fatalf("failed with correct port: %v", err)
	}
	if res == nil || res.start != 22 || res.end != 22 {
		t.Fatalf("readPortRange mistake handles single port range, result: %#v", res)
	}

	t.Log("> testing port range")

	t.Log("> > testing correct port range")
	res, err = readPortRange("0-65535")
	if err != nil {
		t.Fatalf("failed with correct port range: %v", err)
	}
	if res == nil || res.start != 0 || res.end != 65535 {
		t.Fatalf("readPortRange mistake handles normal port range, result: %#v", res)
	}

	t.Log("> > testing correct port range (start==end)")
	res, err = readPortRange("22-22")
	if err != nil {
		t.Fatalf("failed with correct port range (start==end): %v", err)
	}
	if res == nil || res.start != 22 || res.end != 22 {
		t.Fatalf("readPortRange mistake normal port range (start==end), result: %#v", res)
	}

	t.Log("> > testing inverse range port")
	res, err = readPortRange("22-2")
	if err == nil {
		t.Fatal("didn't found inverse range port")
	}
	t.Logf("----successfully found inverse range port: %v", err)

	t.Log("> > testing negative end port")
	res, err = readPortRange("22--22")
	if err == nil {
		t.Fatal("didn't found negative end port")
	}
	t.Logf("----successfully found negative end port: %v", err)

	t.Log("> > testing too large end port")
	res, err = readPortRange("22-65536")
	if err == nil {
		t.Fatal("didn't found too large end port")
	}
	t.Logf("----successfully found too large end port: %v", err)
}

func TestReadPortMapping(t *testing.T) {
	var (
		res *_PortMapping
		err error
	)

	t.Log("> testing normal one to one map")
	res, err = readPortMapping(&PortMapping{
		HostPort:      "80",
		ContainerPort: "3000",
		Protocol:      "tcp",
	})
	if err != nil {
		t.Fatalf("failed to read single port mapping: %v", err)
	}
	if res == nil || res.host == nil || res.container == nil ||
		res.host.start != 80 || res.host.end != 80 ||
		res.container.start != 3000 || res.container.end != 3000 ||
		res.protocol != "tcp" {
		t.Fatalf("mistaken read port mapping as h: %#v; c: %#v, p:%#v", res.host, res.container, res.protocol)
	}

	t.Log("> testing normal multi to one map (and default proto tcp)")
	res, err = readPortMapping(&PortMapping{
		HostPort:      "80-88",
		ContainerPort: "3000",
		Protocol:      "",
	})
	if err != nil {
		t.Fatalf("failed to read N-1 port mapping: %v", err)
	}
	if res == nil || res.host == nil || res.container == nil ||
		res.host.start != 80 || res.host.end != 88 ||
		res.container.start != 3000 || res.container.end != 3000 ||
		res.protocol != "tcp" {
		t.Fatalf("mistaken read N:1 port mapping as h: %#v; c: %#v, p:%#v", res.host, res.container, res.protocol)
	}

	t.Log("> testing normal multi to multi map")
	res, err = readPortMapping(&PortMapping{
		HostPort:      "80-88",
		ContainerPort: "3000-3008",
		Protocol:      "tcp",
	})
	if err != nil {
		t.Fatalf("failed to read N:N port mapping: %v", err)
	}
	if res == nil || res.host == nil || res.container == nil ||
		res.host.start != 80 || res.host.end != 88 ||
		res.container.start != 3000 || res.container.end != 3008 ||
		res.protocol != "tcp" {
		t.Fatalf("mistaken read port mapping as h: %#v; c: %#v, p:%#v", res.host, res.container, res.protocol)
	}

	t.Log("> testing mismatch map")
	res, err = readPortMapping(&PortMapping{
		HostPort:      "80-88",
		ContainerPort: "3000-3010",
		Protocol:      "tcp",
	})
	if err == nil {
		t.Fatal("not found N:M port mapping")
	}
	t.Logf("--found N:M port mapping: %v", err)

	t.Log("> testing 1:N map")
	res, err = readPortMapping(&PortMapping{
		HostPort:      "80",
		ContainerPort: "3000-3010",
		Protocol:      "tcp",
	})
	if err == nil {
		t.Fatal("not found 1:N port mapping")
	}
	t.Logf("--found 1:N port mapping: %v", err)

	t.Log("> testing map wrong range")
	res, err = readPortMapping(&PortMapping{
		HostPort:      "80--88",
		ContainerPort: "3000-3010",
		Protocol:      "tcp",
	})
	if err == nil {
		t.Fatal("not found host port range fault")
	}
	t.Logf("--found host port range fault: %v", err)

	t.Log("> testing map wrong range")
	res, err = readPortMapping(&PortMapping{
		HostPort:      "80-88",
		ContainerPort: "3020-3010",
		Protocol:      "tcp",
	})
	if err == nil {
		t.Fatal("not found container port range fault")
	}
	t.Logf("--found container port range fault: %v", err)

	t.Log("> testing illegal protocol")
	res, err = readPortMapping(&PortMapping{
		HostPort:      "80",
		ContainerPort: "3000",
		Protocol:      "sctp",
	})
	if err == nil {
		t.Fatal("not found illegal protocol")
	}
	t.Logf("--found illegal protocol: %v", err)
}

func TestMergePorts(t *testing.T) {
	var (
		pms []*_PortMapping
		res []*_PortMapping
		err error
	)

	t.Log("> testing empty port mapping list")
	pms = []*_PortMapping{}
	res, err = mergeContinuousPorts(pms)
	if err != nil {
		t.Fatalf("failed with emport pm list: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("non empty output on empty input %v", res)
	}

	t.Log("> testing sparse merge")
	pms = []*_PortMapping{
		{
			host:      &_PortRange{start: 8000, end: 8000},
			container: &_PortRange{start: 8000, end: 8000},
		},
		{
			host:      &_PortRange{start: 8010, end: 8010},
			container: &_PortRange{start: 8010, end: 8010},
		},
		{
			host:      &_PortRange{start: 8020, end: 8030},
			container: &_PortRange{start: 8020, end: 8030},
		},
	}
	t.Logf("---[debug] input items %v", pms)
	res, err = mergeContinuousPorts(pms)
	if err != nil {
		t.Fatalf("error with normal pm list: %v", err)
	}
	if len(res) != 3 {
		t.Fatalf("failed with normal pm list: %v", res)
	}
	t.Logf("---[debug] result items %v", res)

	t.Log("> testing connected merge")
	pms = []*_PortMapping{
		{
			host:      &_PortRange{start: 8000, end: 8000},
			container: &_PortRange{start: 8000, end: 8000},
		},
		{
			host:      &_PortRange{start: 8001, end: 8001},
			container: &_PortRange{start: 8001, end: 8001},
		},
		{
			host:      &_PortRange{start: 8002, end: 8030},
			container: &_PortRange{start: 8002, end: 8030},
		},
	}
	t.Logf("---[debug] input items %v", pms)
	res, err = mergeContinuousPorts(pms)
	if err != nil {
		t.Fatalf("error with normal pm list: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("failed with normal pm list: %v", res)
	}
	t.Logf("---[debug] result items %v", res)

	pms = []*_PortMapping{
		{
			host:      &_PortRange{start: 8000, end: 8000},
			container: &_PortRange{start: 8000, end: 8000},
		},
		{
			host:      &_PortRange{start: 8001, end: 8001},
			container: &_PortRange{start: 8080, end: 8080},
		},
		{
			host:      &_PortRange{start: 8002, end: 8030},
			container: &_PortRange{start: 8002, end: 8030},
		},
	}
	t.Logf("---[debug] input items %v", pms)
	res, err = mergeContinuousPorts(pms)
	if err != nil {
		t.Fatalf("error with normal pm list: %v", err)
	}
	if len(res) != 3 {
		t.Fatalf("failed with normal pm list: %v", res)
	}
	t.Logf("---[debug] result items %v", res)

	t.Log("> testing overlap merge")
	pms = []*_PortMapping{
		{
			host:      &_PortRange{start: 8000, end: 8000},
			container: &_PortRange{start: 8000, end: 8000},
		},
		{
			host:      &_PortRange{start: 8001, end: 8001},
			container: &_PortRange{start: 8001, end: 8001},
		},
		{
			host:      &_PortRange{start: 8001, end: 8030},
			container: &_PortRange{start: 8001, end: 8030},
		},
	}
	t.Logf("---[debug] input items %v", pms)
	res, err = mergeContinuousPorts(pms)
	if err == nil {
		t.Fatalf("failed with overlapped pm list: %v", res)
	}
	t.Logf("---[debug] found overlapped: %v", err)

	pms = []*_PortMapping{
		{
			host:      &_PortRange{start: 8000, end: 8000},
			container: &_PortRange{start: 8000, end: 8000},
		},
		{
			host:      &_PortRange{start: 8000, end: 8000},
			container: &_PortRange{start: 8080, end: 8080},
		},
		{
			host:      &_PortRange{start: 8001, end: 8030},
			container: &_PortRange{start: 8001, end: 8030},
		},
	}
	t.Logf("---[debug] input items %v", pms)
	res, err = mergeContinuousPorts(pms)
	if err == nil {
		t.Fatalf("failed with overlapped pm list: %v", res)
	}
	t.Logf("---[debug] found overlapped: %v", err)

	pms = []*_PortMapping{
		{
			host:      &_PortRange{start: 8080, end: 8080},
			container: &_PortRange{start: 8080, end: 8080},
		},
		{
			host:      &_PortRange{start: 8000, end: 8000},
			container: &_PortRange{start: 8000, end: 8000},
		},
		{
			host:      &_PortRange{start: 8001, end: 8090},
			container: &_PortRange{start: 8001, end: 8090},
		},
	}
	t.Logf("---[debug] input items %v", pms)
	res, err = mergeContinuousPorts(pms)
	if err == nil {
		t.Fatalf("failed with overlapped pm list: %v", res)
	}
	t.Logf("---[debug] found overlapped: %v", err)

	t.Log("> testing random port")
	pms = []*_PortMapping{
		{
			host:      &_PortRange{start: 8000, end: 8000},
			container: &_PortRange{start: 8000, end: 8000},
		},
		{
			host:      &_PortRange{start: 8001, end: 8001},
			container: &_PortRange{start: 8001, end: 8001},
		},
		{
			host:      &_PortRange{start: 8000, end: 8003},
			container: &_PortRange{start: 8002, end: 8002},
		},
	}
	t.Logf("---[debug] input items %v", pms)
	res, err = mergeContinuousPorts(pms)
	if err != nil {
		t.Fatalf("error with random pm list: %v", err)
	}
	if len(res) > 2 {
		t.Fatalf("failed with random pm list: %v", res)
	}
	t.Logf("---[debug] result items %v", res)

	pms = []*_PortMapping{
		{
			host:      &_PortRange{start: 8000, end: 8000},
			container: &_PortRange{start: 8000, end: 8000},
		},
		{
			host:      &_PortRange{start: 8001, end: 8001},
			container: &_PortRange{start: 8001, end: 8001},
		},
		{
			host:      &_PortRange{start: 8000, end: 8001},
			container: &_PortRange{start: 8001, end: 8001},
		},
	}
	t.Logf("---[debug] input items %v", pms)
	res, err = mergeContinuousPorts(pms)
	if err == nil {
		t.Fatalf("didn't found collision in random pm list: %v", res)
	}
	t.Logf("found error with random pm list: %v", err)

	pms = []*_PortMapping{
		{
			host:      &_PortRange{start: 8000, end: 8000},
			container: &_PortRange{start: 8000, end: 8000},
		},
		{
			host:      &_PortRange{start: 8001, end: 8001},
			container: &_PortRange{start: 8001, end: 8001},
		},
		{
			host:      &_PortRange{start: 8000, end: 8003},
			container: &_PortRange{start: 8003, end: 8003},
		},
		{
			host:      &_PortRange{start: 8001, end: 8002},
			container: &_PortRange{start: 8002, end: 8002},
		},
	}
	t.Logf("---[debug] input items %v", pms)
	res, err = mergeContinuousPorts(pms)
	if err != nil {
		t.Logf("got an error with random pm list: %v", err)
		t.Log("it's acceptable to fail in this case")
	}
	if len(res) > 3 {
		t.Fatalf("failed with random pm list: %v", res)
	}
	t.Logf("---[debug] result items %v", res)
}

func TestContainerPortMigrate(t *testing.T) {
	var (
		tp  = &UserPod{}
		err error
	)

	t.Log("> testing nil containers")
	err = tp.migrateContainerPorts()
	if err != nil {
		t.Fatal("failed to migrate nil containers")
	}

	t.Log("> testing normal migrate")
	tp.Containers = []*UserContainer{
		{
			Ports: []*UserContainerPort{
				{
					HostPort:      80,
					ContainerPort: 80,
				},
				{
					HostPort:      81,
					ContainerPort: 81,
				},
			},
		},
	}
	err = tp.migrateContainerPorts()
	if err != nil {
		t.Fatalf("failed to migrate container ports: %v", err)
	}

	t.Log("> testing Illegal proto")
	tp.Containers = []*UserContainer{
		{
			Ports: []*UserContainerPort{
				{
					HostPort:      80,
					ContainerPort: 80,
					Protocol:      "icmp",
				},
				{
					HostPort:      81,
					ContainerPort: 81,
				},
			},
		},
	}
	err = tp.migrateContainerPorts()
	if err == nil {
		t.Fatalf("failed to find illegal container port protocol: %v", tp.Portmappings)
	}
	t.Logf("found illegal proto: %v", err)
}

func TestMergePortMappings(t *testing.T) {
	var (
		tp  = &UserPod{}
		err error
	)

	t.Log("> testing nil port mappings")
	err = tp.MergePortmappings()
	if err != nil {
		t.Fatalf("failed with nil port mappings rules: %v", err)
	}

	t.Log("> testing empty port mappings")
	tp.Portmappings = []*PortMapping{}
	err = tp.MergePortmappings()
	if err != nil {
		t.Fatalf("failed with empty port mappings rules: %v", err)
	}

	t.Log("> testing normal trans")
	tp.Portmappings = []*PortMapping{
		{
			HostPort:      "8000",
			ContainerPort: "8000",
		},
		{
			HostPort:      "8010",
			ContainerPort: "8010",
		},
		{
			HostPort:      "8020-8030",
			ContainerPort: "8020-8030",
			Protocol:      "udp",
		},
	}
	err = tp.MergePortmappings()
	if err != nil {
		t.Fatalf("failed with normal port mappings rules: %v", err)
	}

	t.Log("> testing fail container ports migrate")
	tp.Portmappings = nil
	tp.Containers = []*UserContainer{
		{
			Ports: []*UserContainerPort{
				{
					HostPort:      80,
					ContainerPort: 80,
					Protocol:      "icmp",
				},
				{
					HostPort:      81,
					ContainerPort: 81,
				},
			},
		},
	}
	err = tp.MergePortmappings()
	if err == nil {
		t.Fatalf("failed with failed container ports migrate rules: %v", tp.Portmappings)
	}

	t.Log("> testing illegal proto")
	tp.Portmappings = []*PortMapping{
		{
			HostPort:      "8000",
			ContainerPort: "8000",
		},
		{
			HostPort:      "8010",
			ContainerPort: "8010",
		},
		{
			HostPort:      "8020-8030",
			ContainerPort: "8020-8030",
			Protocol:      "icmp",
		},
	}
	err = tp.MergePortmappings()
	if err == nil {
		t.Fatalf("failed with illegal proto: %#v", tp.Portmappings)
	}

	t.Log("> testing tcp merge failed")
	tp.Portmappings = []*PortMapping{
		{
			HostPort:      "8000",
			ContainerPort: "8000",
		},
		{
			HostPort:      "8000",
			ContainerPort: "8010",
		},
		{
			HostPort:      "8020-8030",
			ContainerPort: "8020-8030",
			Protocol:      "udp",
		},
	}
	err = tp.MergePortmappings()
	if err == nil {
		t.Fatalf("failed with tcp port overlapped rules: %v", tp.Portmappings)
	}

	t.Log("> testing udp merge failed")
	tp.Portmappings = []*PortMapping{
		{
			HostPort:      "8000",
			ContainerPort: "8000",
		},
		{
			HostPort:      "8023",
			ContainerPort: "8010",
			Protocol:      "udp",
		},
		{
			HostPort:      "8020-8030",
			ContainerPort: "8020-8030",
			Protocol:      "udp",
		},
	}
	err = tp.MergePortmappings()
	if err == nil {
		t.Fatalf("failed with udp port overlapped rules: %v", tp.Portmappings)
	}
}
