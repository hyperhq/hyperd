package hypervisor

import (
	"encoding/json"
	"github.com/hyperhq/hyper/pod"
	"testing"
)

func TestInitContext(t *testing.T) {

	dr := &EmptyDriver{}
	dr.Initialize()

	b := &BootConfig{
		CPU:    3,
		Memory: 202,
		Kernel: "somekernel",
		Initrd: "someinitrd",
	}

	ctx, _ := InitContext(dr, "vmid", nil, nil, nil, b)

	if ctx.Id != "vmid" {
		t.Error("id should be vmid, but is ", ctx.Id)
	}
	if ctx.Boot.CPU != 3 {
		t.Error("cpu should be 3, but is ", string(ctx.Boot.CPU))
	}
	if ctx.Boot.Memory != 202 {
		t.Error("memory should be 202, but is ", string(ctx.Boot.Memory))
	}

	t.Log("id check finished.")
	ctx.Close()
}

func TestParseSpec(t *testing.T) {

	dr := &EmptyDriver{}
	dr.Initialize()

	b := &BootConfig{
		CPU:    1,
		Memory: 128,
		Kernel: "somekernel",
		Initrd: "someinitrd",
	}

	cs := []*ContainerInfo{
		&ContainerInfo{},
	}

	ctx, _ := InitContext(dr, "vmid", nil, nil, nil, b)

	spec := pod.UserPod{}
	err := json.Unmarshal([]byte(testJson("basic")), &spec)
	if err != nil {
		t.Error("parse json failed ", err.Error())
	}

	ctx.InitDeviceContext(&spec, cs, nil)

	if ctx.userSpec != &spec {
		t.Error("user pod assignment fail")
	}

	if len(ctx.vmSpec.Containers) != 1 {
		t.Error("wrong containers in vm spec")
	}

	if ctx.vmSpec.ShareDir != "share_dir" {
		t.Error("shareDir in vmSpec is ", ctx.vmSpec.ShareDir)
	}

	if ctx.vmSpec.Containers[0].RestartPolicy != "never" {
		t.Error("Default restartPolicy is ", ctx.vmSpec.Containers[0].RestartPolicy)
	}

	if ctx.vmSpec.Containers[0].Envs[1].Env != "JAVA_HOME" {
		t.Error("second environment should not be ", ctx.vmSpec.Containers[0].Envs[1].Env)
	}

	res, err := json.MarshalIndent(*ctx.vmSpec, "    ", "    ")
	if err != nil {
		t.Error("vmspec to json failed")
	}
	t.Log(string(res))
}

func TestParseVolumes(t *testing.T) {
	dr := &EmptyDriver{}
	dr.Initialize()

	b := &BootConfig{
		CPU:    1,
		Memory: 128,
		Kernel: "somekernel",
		Initrd: "someinitrd",
	}

	ctx, _ := InitContext(dr, "vmid", nil, nil, nil, b)

	spec := pod.UserPod{}
	err := json.Unmarshal([]byte(testJson("with_volumes")), &spec)
	if err != nil {
		t.Error("parse json failed ", err.Error())
	}

	cs := []*ContainerInfo{
		&ContainerInfo{},
	}

	ctx.InitDeviceContext(&spec, cs, nil)

	res, err := json.MarshalIndent(*ctx.vmSpec, "    ", "    ")
	if err != nil {
		t.Error("vmspec to json failed")
	}
	t.Log(string(res))

	vol1 := ctx.devices.volumeMap["vol1"]
	if vol1.pos[0] != "/var/dir1" {
		t.Error("vol1 (/var/dir1) path is ", vol1.pos[0])
	}

	if !vol1.readOnly[0] {
		t.Error("vol1 on container 0 should be read only")
	}

	ref1 := blockDescriptor{name: "vol1", filename: "", format: "", fstype: "", deviceName: ""}
	if *vol1.info != ref1 {
		t.Errorf("info of vol1: %q %q %q %q %q",
			vol1.info.name, vol1.info.filename, vol1.info.format, vol1.info.fstype, vol1.info.deviceName)
	}

	vol2 := ctx.devices.volumeMap["vol2"]
	if vol2.pos[0] != "/var/dir2" {
		t.Error("vol1 (/var/dir2) path is ", vol2.pos[0])
	}

	if vol2.readOnly[0] {
		t.Error("vol2 on container 0 should not be read only")
	}

	ref2 := blockDescriptor{name: "vol2", filename: "/home/whatever", format: "vfs", fstype: "dir", deviceName: ""}
	if *vol2.info != ref2 {
		t.Errorf("info of vol2: %q %q %q %q %q",
			vol2.info.name, vol2.info.filename, vol2.info.format, vol2.info.fstype, vol2.info.deviceName)
	}
}

func testJson(key string) string {
	jsons := make(map[string]string)

	jsons["basic"] = `{
    "name": "hostname",
    "containers" : [{
        "image": "nginx:latest",
        "files":  [{
            "path": "/var/lib/xxx/xxxx",
            "filename": "filename"
        }],
        "envs":  [{
            "env": "JAVA_OPT",
            "value": "-XMx=256m"
        },{
            "env": "JAVA_HOME",
            "value": "/usr/local/java"
        }]
    }],
    "resource": {
        "vcpu": 1,
        "memory": 128
    },
    "files": [{
        "name": "filename",
        "encoding": "raw",
        "uri": "https://s3.amazonaws/bucket/file.conf",
        "content": ""
    }],
    "volumes": []}`

	jsons["with_volumes"] = `{
    "name": "hostname",
    "containers" : [{
        "image": "nginx:latest",
        "files":  [{
            "path": "/var/lib/xxx/xxxx",
            "filename": "filename"
        }],
        "volumes": [{
            "path": "/var/dir1",
            "volume": "vol1",
            "readOnly": true
        },{
            "path": "/var/dir2",
            "volume": "vol2",
            "readOnly": false
        },{
            "path": "/var/dir3",
            "volume": "vol3",
            "readOnly": false
        },{
            "path": "/var/dir4",
            "volume": "vol4",
            "readOnly": false
        },{
            "path": "/var/dir5",
            "volume": "vol5",
            "readOnly": false
        },{
            "path": "/var/dir6",
            "volume": "vol6",
            "readOnly": false
        }]
    }],
    "resource": {
        "vcpu": 1,
        "memory": 128
    },
    "files": [],
    "volumes": [{
        "name": "vol1",
        "source": "",
        "driver": ""
    },{
        "name": "vol2",
        "source": "/home/whatever",
        "driver": "vfs"
    },{
        "name": "vol3",
        "source": "/home/what/file",
        "driver": "raw"
    },{
        "name": "vol4",
        "source": "",
        "driver": ""
    },{
        "name": "vol5",
        "source": "/home/what/file2",
        "driver": "vfs"
    },{
        "name": "vol6",
        "source": "/home/what/file3",
        "driver": "qcow2"
    }]
    }`

	return jsons[key]
}
