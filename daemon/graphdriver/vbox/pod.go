package vbox

import (
	"fmt"
	"os"

	"github.com/hyperhq/hyperd/utils"
	"github.com/hyperhq/runv/hypervisor/pod"
)

func MakeDiffPod(podName, image, id, srcDisk, tgtDisk, outDir string) (string, error) {
	if image == "" || srcDisk == "" {
		return "", fmt.Errorf("image/source diff can not be null")
	}
	var (
		env           = []pod.UserEnvironmentVar{}
		containerList = []pod.UserContainer{}
		volList       = []pod.UserVolume{}
	)

	var cVols = []pod.UserVolumeReference{}
	var cVol1 = pod.UserVolumeReference{
		Path:     "/tmp/srcdisk/",
		Volume:   "srcdisk",
		ReadOnly: false,
	}
	var cVol2 = pod.UserVolumeReference{
		Path:     "/tmp/tgtdisk/",
		Volume:   "tgtdisk",
		ReadOnly: false,
	}
	var cVol3 = pod.UserVolumeReference{
		Path:     "/tmp/result/",
		Volume:   "result",
		ReadOnly: false,
	}
	cVols = append(cVols, cVol1)
	cVols = append(cVols, cVol2)
	cVols = append(cVols, cVol3)

	var container = pod.UserContainer{
		Name:          "mac-diff-disk",
		Image:         image,
		Command:       []string{"/bin/hyperdiff", "--layer", "/tmp/srcdisk/rootfs/", "--parent", "/tmp/tgtdisk/rootfs/", "--tar", "/tmp/result/" + id + ".tar"},
		Workdir:       "/",
		Entrypoint:    []string{},
		Ports:         []pod.UserContainerPort{},
		Envs:          env,
		Volumes:       cVols,
		Files:         []pod.UserFileReference{},
		RestartPolicy: "never",
	}
	containerList = append(containerList, container)

	var vol1 = pod.UserVolume{
		Name:   "srcdisk",
		Source: srcDisk,
		Driver: "vdi",
	}
	var vol2 pod.UserVolume
	if tgtDisk != "" {
		vol2 = pod.UserVolume{
			Name:   "tgtdisk",
			Source: tgtDisk,
			Driver: "vdi",
		}
	} else {
		if err := os.MkdirAll("/var/tmp/hyper/nulldisk/rootfs/", 0755); err != nil {
			return "", err
		}
		vol2 = pod.UserVolume{
			Name:   "tgtdisk",
			Source: "/var/tmp/hyper/nulldisk",
			Driver: "vfs",
		}
	}
	var vol3 = pod.UserVolume{
		Name:   "result",
		Source: outDir,
		Driver: "vfs",
	}
	volList = append(volList, vol1)
	volList = append(volList, vol2)
	volList = append(volList, vol3)

	var userPod = &pod.UserPod{
		Name:       podName,
		Containers: containerList,
		Resource:   pod.UserResource{Vcpu: 1, Memory: 64},
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

func MakeMountPod(podName, image, id, diffSrc, volDst string) (string, error) {
	if image == "" || diffSrc == "" || volDst == "" {
		return "", fmt.Errorf("image/source diff/target vol can not be null")
	}
	var (
		env           = []pod.UserEnvironmentVar{}
		containerList = []pod.UserContainer{}
		volList       = []pod.UserVolume{}
	)

	var cVols = []pod.UserVolumeReference{}
	var cVol1 = pod.UserVolumeReference{
		Path:     "/tmp/image",
		Volume:   "image",
		ReadOnly: false,
	}
	var cVol2 = pod.UserVolumeReference{
		Path:     "/tmp/imagecontent",
		Volume:   "imagecontent",
		ReadOnly: false,
	}
	var cVol3 = pod.UserVolumeReference{
		Path:     "/tmp/error",
		Volume:   "error",
		ReadOnly: false,
	}
	cVols = append(cVols, cVol1)
	cVols = append(cVols, cVol2)
	cVols = append(cVols, cVol3)

	var container = pod.UserContainer{
		Name:          "mac-mount-disk",
		Image:         image,
		Command:       []string{"/pull-image.sh"},
		Workdir:       "/",
		Entrypoint:    []string{},
		Ports:         []pod.UserContainerPort{},
		Envs:          env,
		Volumes:       cVols,
		Files:         []pod.UserFileReference{},
		RestartPolicy: "never",
	}
	containerList = append(containerList, container)

	var vol1 = pod.UserVolume{
		Name:   "imagecontent",
		Source: diffSrc,
		Driver: "vfs",
	}
	var vol2 = pod.UserVolume{
		Name:   "image",
		Source: volDst,
		Driver: "vdi",
	}
	var vol3 = pod.UserVolume{
		Name:   "error",
		Source: "/tmp/error/",
		Driver: "vfs",
	}
	volList = append(volList, vol1)
	volList = append(volList, vol2)
	volList = append(volList, vol3)

	var userPod = &pod.UserPod{
		Name:       podName,
		Containers: containerList,
		Resource:   pod.UserResource{Vcpu: 1, Memory: 64},
		Files:      []pod.UserFile{},
		Volumes:    volList,
		Tty:        false,
	}
	os.MkdirAll("/tmp/error/", 0755)
	jsonString, err := utils.JSONMarshal(userPod, true)
	if err != nil {
		return "", err
	}
	return string(jsonString), nil
}
