package pod

import (
    "testing"
)

func TestRandStr(t *testing.T) {
    r1 := RandStr(10, "alphanum")
    r2 := RandStr(10, "alphanum")
    r3 := RandStr(10, "alphanum")

    if r1 == r2 || r1 == r3 || r2 == r3 {
        t.Fatal("The RandStr function should not return same string!")
    }
}

func TestProcessPodFile(t *testing.T) {
    fakeJsonFile := "test.pod"
    if _, err := ProcessPodFile(fakeJsonFile); err == nil {
        t.Fatal("The ProcessPodFile should return an error while getting a non-exist file!\n")
    }
}

func TestProcessPodBytes(t *testing.T) {
    jsonStr := `{ "id": "test-container-create-1", "containers" : [{ "name": "web", "image": "tomcat:latest" }], "resource": { "vcpu": 1, "memory": 512 }, "files": [], "volumes": [] }`
    if _, err := ProcessPodBytes([]byte(jsonStr)); err != nil {
        t.Fatal("The ProcessPodBytes function return an error while processing a right json string!")
    }

    jsonStrWithoutId := `{ "id": "", "containers" : [{ "name": "web", "image": "tomcat:latest" }], "resource": { "vcpu": 1, "memory": 512 }, "files": [], "volumes": [] }`
    userPod, err := ProcessPodBytes([]byte(jsonStrWithoutId))
    if err != nil {
        t.Fatal("The ProcessPodBytes function return an error while processing a right json string without id name!")
    }
    if userPod.Name == "" {
        t.Fatal("The ProcessPodBytes function do not create an ID name for that pod!\n")
    }
    jsonStrWithoutName := `{ "id": "test-container-create-1", "containers" : [{ "name": "web", "image": "" }], "resource": { "vcpu": 1, "memory": 512 }, "files": [], "volumes": [] }`
    userPod, err = ProcessPodBytes([]byte(jsonStrWithoutName))
    if err == nil {
        t.Fatal("The ProcessPodBytes function should return an error while processing a json string without image name!")
    }
}
