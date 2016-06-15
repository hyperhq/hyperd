package integration

import (
	"testing"

	"github.com/hyperhq/hyperd/types"
	. "gopkg.in/check.v1"
)

const (
	server = "127.0.0.1:22318"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { TestingT(t) }

type TestSuite struct {
	client *HyperClient
}

var _ = Suite(&TestSuite{})

func (s *TestSuite) SetUpSuite(c *C) {
	cl, err := NewHyperClient(server)
	c.Assert(err, IsNil)
	if err != nil {
		c.Skip("hyperd is down")
	}
	s.client = cl
}

func (s *TestSuite) TestGetPodList(c *C) {
	podList, err := s.client.GetPodList()
	c.Assert(err, IsNil)
	c.Logf("Got PodList %v", podList)
}

func (s *TestSuite) TestGetVMList(c *C) {
	vmList, err := s.client.GetVMList()
	c.Assert(err, IsNil)
	c.Logf("Got VMList %v", vmList)
}

func (s *TestSuite) TestGetContainerList(c *C) {
	containerList, err := s.client.GetContainerList(true)
	c.Assert(err, IsNil)
	c.Logf("Got ContainerList %v", containerList)
}

func (s *TestSuite) TestGetImageList(c *C) {
	imageList, err := s.client.GetImageList()
	c.Assert(err, IsNil)
	c.Logf("Got ImageList %v", imageList)
}

func (s *TestSuite) TestGetContainerInfo(c *C) {
	containerList, err := s.client.GetContainerList(true)
	c.Assert(err, IsNil)
	c.Logf("Got ContainerList %v", containerList)

	if len(containerList) == 0 {
		return
	}

	info, err := s.client.GetContainerInfo(containerList[0].ContainerID)
	c.Assert(err, IsNil)
	c.Logf("Got ContainerInfo %v", info)
}

func (s *TestSuite) TestGetContainerLogs(c *C) {
	containerList, err := s.client.GetContainerList(true)
	c.Assert(err, IsNil)
	c.Logf("Got ContainerList %v", containerList)

	if len(containerList) == 0 {
		return
	}

	logs, err := s.client.GetContainerLogs(containerList[0].ContainerID)
	c.Assert(err, IsNil)
	c.Logf("Got ContainerLogs %v", logs)
}

func (s *TestSuite) TestPostAttach(c *C) {
	err := s.client.PullImage("busybox", "latest", nil)
	c.Assert(err, IsNil)

	spec := types.UserPod{
		Containers: []*types.UserContainer{
			{
				Image: "busybox",
			},
		},
	}

	pod, err := s.client.CreatePod(&spec)
	c.Assert(err, IsNil)
	c.Logf("Pod created: %s", pod)
	defer s.client.RemovePod(pod)

	err = s.client.StartPod(pod, "", "")
	c.Assert(err, IsNil)

	podInfo, err := s.client.GetPodInfo(pod)
	c.Assert(err, IsNil)

	err = s.client.PostAttach(podInfo.Status.ContainerStatus[0].ContainerID, "abcdefgh")
	c.Assert(err, IsNil)
}

func (s *TestSuite) TestGetPodInfo(c *C) {
	podList, err := s.client.GetPodList()
	c.Assert(err, IsNil)
	c.Logf("Got PodList %v", podList)

	if len(podList) == 0 {
		return
	}

	info, err := s.client.GetPodInfo(podList[0].PodID)
	c.Assert(err, IsNil)
	c.Logf("Got PodInfo %v", info)
}

func (s *TestSuite) TestGetVMCreateRemove(c *C) {
	vm, err := s.client.CreateVM(1, 64)
	c.Assert(err, IsNil)

	var found = false
	vmList, err := s.client.GetVMList()
	c.Assert(err, IsNil)
	c.Logf("Got VMList %v", vmList)
	for _, v := range vmList {
		if v.VmID == vm {
			found = true
		}
	}
	if !found {
		c.Errorf("Can't find vm %s", vm)
	}

	resp, err := s.client.RemoveVM(vm)
	c.Assert(err, IsNil)
	c.Logf("RemoveVM resp %s", resp.String())
}

func (s *TestSuite) TestCreatePod(c *C) {
	err := s.client.PullImage("busybox", "latest", nil)
	c.Assert(err, IsNil)

	spec := types.UserPod{
		Id: "busybox",
		Containers: []*types.UserContainer{
			{
				Image: "busybox",
			},
		},
	}

	pod, err := s.client.CreatePod(&spec)
	c.Assert(err, IsNil)
	c.Logf("Pod created: %s", pod)

	podList, err := s.client.GetPodList()
	c.Assert(err, IsNil)
	c.Logf("Got PodList %v", podList)

	var found = false
	for _, p := range podList {
		if p.PodID == pod {
			found = true
			break
		}
	}
	if !found {
		c.Errorf("Can't found pod %s", pod)
	}

	err = s.client.RemovePod(pod)
	c.Assert(err, IsNil)
}

func (s *TestSuite) TestStartPod(c *C) {
	podList, err := s.client.GetPodList()
	c.Assert(err, IsNil)
	c.Logf("Got PodList %v", podList)

	if len(podList) == 0 {
		return
	}

	err = s.client.StartPod(podList[0].PodID, "", "")
	c.Assert(err, IsNil)

	podInfo, err := s.client.GetPodInfo(podList[0].PodID)
	c.Assert(err, IsNil)
	c.Assert(podInfo.Status.Phase, Equals, "Running")
}

func (s *TestSuite) TestCreateContainer(c *C) {
	err := s.client.PullImage("busybox", "latest", nil)
	c.Assert(err, IsNil)

	spec := types.UserPod{}
	pod, err := s.client.CreatePod(&spec)
	c.Assert(err, IsNil)
	c.Logf("Pod created: %s", pod)

	container, err := s.client.CreateContainer(pod, &types.UserContainer{
		Image: "busybox",
	})
	c.Assert(err, IsNil)
	c.Logf("Container created: %s", container)

	info, err := s.client.GetContainerInfo(container)
	c.Assert(err, IsNil)
	c.Assert(info.PodID, Equals, pod)

	err = s.client.RemovePod(pod)
	c.Assert(err, IsNil)
}

func (s *TestSuite) TestPullImage(c *C) {
	err := s.client.PullImage("alpine", "latest", nil)
	c.Assert(err, IsNil)

	list, err := s.client.GetImageList()
	c.Assert(err, IsNil)
	found := false
	for _, img := range list {
		for _, repo := range img.RepoTags {
			if repo == "alpine:latest" {
				found = true
				break
			}
		}
	}
	c.Assert(found, Equals, true)

	err = s.client.RemoveImage("alpine")
	c.Assert(err, IsNil)
	list, err = s.client.GetImageList()
	c.Assert(err, IsNil)

	found = false
	for _, img := range list {
		for _, repo := range img.RepoTags {
			if repo == "alpine:latest" {
				found = true
				break
			}
		}
	}
	c.Assert(found, Equals, false)
}

func (s *TestSuite) TestAddListDeleteService(c *C) {
	spec := types.UserPod{
		Containers: []*types.UserContainer{
			{
				Image:   "busybox",
				Command: []string{"sleep", "10000"},
			},
		},
		Services: []*types.UserService{
			&types.UserService{
				ServiceIP:   "10.254.0.24",
				ServicePort: 2834,
				Protocol:    "TCP",
				Hosts: []*types.UserServiceBackend{
					&types.UserServiceBackend{
						HostIP:   "192.168.23.2",
						HostPort: 2345,
					},
				},
			},
		},
	}

	pod, err := s.client.CreatePod(&spec)
	c.Assert(err, IsNil)
	c.Logf("Pod created: %v", pod)

	// clear the test pod
	defer func() {
		err = s.client.RemovePod(pod)
		c.Assert(err, IsNil)
	}()

	err = s.client.StartPod(pod, "", "")
	c.Assert(err, IsNil)

	// testService := "[{\"serviceip\": \"10.10.0.22\",\"serviceport\": 80,\"protocol\": \"TCP\",\"hosts\": [{\"hostip\": \"192.168.23.2\",\"hostport\": 8080}]}]"
	// err = s.client.AddService(pod, testService)
	// c.Assert(err, IsNil)

	svcList, err := s.client.ListService(pod)
	c.Assert(err, IsNil)
	c.Assert(len(svcList), Equals, 2)

	// err = s.client.DeleteService(pod, testService)
	// c.Assert(err, IsNil)
	// svcList, err = s.client.ListService(pod)
	// c.Assert(len(svcList), Equals, 1)
}
