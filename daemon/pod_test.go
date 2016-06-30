package daemon

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hyperhq/runv/hypervisor/pod"
)

func TestDNSInsertRegular(t *testing.T) {
	type spec_check func(*pod.UserPod) bool
	var (
		inputs = map[string]string{
			"empty":      "",
			"normal":     `{"id":"ubuntu-2929430143","containers":[{"name":"ubuntu-2929430143","image":"ubuntu","command":["/bin/bash"],"workdir":"/","entrypoint":[],"ports":[],"envs":[],"volumes":[],"files":[],"restartPolicy":"never"}],"resource":{"vcpu":1,"memory":128},"files":[],"volumes":[],"log":{"type":"","config":{}},"tty":true,"type":"", "RestartPolicy":""}`,
			"configured": `{"id":"ubuntu-2929430143","dns":["8.8.8.8"],"containers":[{"name":"ubuntu-2929430143","image":"ubuntu","command":["/bin/bash"],"workdir":"/","entrypoint":[],"ports":[],"envs":[],"volumes":[],"files":[],"restartPolicy":"never"}],"resource":{"vcpu":1,"memory":128},"files":[],"volumes":[],"log":{"type":"","config":{}},"tty":true,"type":"", "RestartPolicy":""}`,
			"skipped":    `{"id":"ubuntu-2929430143","containers":[{"name":"ubuntu-2929430143","image":"ubuntu","command":["/bin/bash"],"workdir":"/","entrypoint":[],"ports":[],"envs":[],"volumes":[],"files":[{"path":"/test/1","filename":"rx","perm":"0600"}],"restartPolicy":"never"}],"resource":{"vcpu":1,"memory":128},"files":[{"name":"rx","encoding":"raw", "uri":"file:///etc/resolv.conf"}],"volumes":[],"log":{"type":"","config":{}},"tty":true,"type":"", "RestartPolicy":""}`,
		}
		errs = map[string]error{
			"empty":      fmt.Errorf("No Spec available for insert a DNS configuration"),
			"normal":     nil,
			"configured": nil,
			"skipped":    nil,
		}
		checks = map[string]spec_check{
			"empty": nil,
			"normal": func(s *pod.UserPod) bool {
				if s == nil || len(s.Files) != 1 || len(s.Containers) != 1 || len(s.Containers[0].Files) != 1 {
					t.Log("structure uncorrect")
					return false
				}
				if s.Files[0].Uri != "file:///etc/resolv.conf" {
					t.Log("Src is not correct")
					return false
				}
				if s.Containers[0].Files[0].Filename != s.Files[0].Name {
					t.Log("file id doesnot match each other")
					return false
				}
				if s.Containers[0].Files[0].Path != "/etc/resolv.conf" {
					t.Log("target is not correct")
					return false
				}
				return true
			},
			"configured": func(s *pod.UserPod) bool {
				if s == nil || len(s.Files) != 0 || len(s.Containers) != 1 || len(s.Containers[0].Files) != 0 {
					t.Log("structure uncorrect")
					return false
				}
				if len(s.Dns) != 1 || s.Dns[0] != "8.8.8.8" {
					t.Log("affected original DNS")
					return false
				}
				return true
			},
			"skipped": func(s *pod.UserPod) bool {
				if s == nil || len(s.Files) != 1 || len(s.Containers) != 1 || len(s.Containers[0].Files) != 1 {
					t.Log("structure uncorrect")
					return false
				}
				if s.Files[0].Uri != "file:///etc/resolv.conf" {
					t.Log("Src is not correct")
					return false
				}
				if s.Files[0].Name != "rx" {
					t.Log("Src file is changed")
					return false
				}
				return true
			},
		}
	)

	for tag, input := range inputs {
		var spec pod.UserPod
		p := &Pod{
			Spec: nil,
		}
		if input != "" {
			_ = json.Unmarshal([]byte(input), &spec)
			p.Spec = &spec
		}
		err := p.setupDNS()
		if (errs[tag] != nil && (err == nil || errs[tag].Error() != err.Error())) || (err != nil && errs[tag] == nil) {
			t.Logf("error should be %v, but is %v", errs[tag], err)
			t.Fail()
		}
		if err != nil {
			continue
		}
		t.Logf("/%s/ got result: %v", tag, p.Spec)
		if !checks[tag](p.Spec) {
			t.Log("process result is not correct")
			t.Fail()
		}
	}
}
