package builder

import (
	"fmt"

	dockerimage "github.com/hyperhq/hyper/lib/docker/image"
	"github.com/hyperhq/hyper/utils"
	"github.com/hyperhq/runv/hypervisor/pod"
)

func MakeBasicPod(podName, image string, cmd []string) (string, error) {
	return MakePod(podName, image, "", "", "", "", cmd, []string{})
}

func MakeCopyPod(podName, image, workdir, src, dst, shellDir string) (string, error) {
	return MakePod(podName, image, workdir, src, dst, shellDir, []string{"/bin/sh", "/tmp/shell/exec-copy.sh"}, []string{})
}

func MakePod(podName, image, workdir, src, vol, shellDir string, cmds, entrys []string) (string, error) {
	if image == "" {
		return "", fmt.Errorf("image can not be null")
	}
	if err := dockerimage.ValidateID(image); err == nil {
		image = image[:12]
	}
	var (
		env           = []pod.UserEnvironmentVar{}
		containerList = []pod.UserContainer{}
		volList       = []pod.UserVolume{}
		cVols         = []pod.UserVolumeReference{}
	)
	if src != "" {
		myVol1 := pod.UserVolumeReference{
			Path:     "/tmp/src/",
			Volume:   "source",
			ReadOnly: false,
		}
		myVol2 := pod.UserVolumeReference{
			Path:     "/tmp/shell/",
			Volume:   "shell",
			ReadOnly: false,
		}
		cVols = append(cVols, myVol1)
		cVols = append(cVols, myVol2)
		vol1 := pod.UserVolume{
			Name:   "source",
			Source: src,
			Driver: "vfs",
		}
		vol2 := pod.UserVolume{
			Name:   "shell",
			Source: shellDir,
			Driver: "vfs",
		}
		volList = append(volList, vol1)
		volList = append(volList, vol2)
	}

	var container = pod.UserContainer{
		Name:          "mac-builder",
		Image:         image,
		Command:       cmds,
		Workdir:       workdir,
		Entrypoint:    entrys,
		Ports:         []pod.UserContainerPort{},
		Envs:          env,
		Volumes:       cVols,
		Files:         []pod.UserFileReference{},
		RestartPolicy: "never",
	}
	containerList = append(containerList, container)

	var userPod = &pod.UserPod{
		Name:       podName,
		Containers: containerList,
		Resource:   pod.UserResource{Vcpu: 1, Memory: 512},
		Files:      []pod.UserFile{},
		Volumes:    volList,
		Tty:        false,
	}

	jsonString, err := utils.JSONMarshal(userPod, true)
	if err != nil {
		return "", err
	}
	return string(jsonString), nil
}
