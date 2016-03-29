package daemondb

import (
	"fmt"
)

const (
	VM_KEY            = "vmdata-%s"
	POD_KEY           = "pod-%s"
	POD_VM_KEY        = "vm-%s"
	POD_CONTAINER_KEY = "pod-container-%s"
	POD_VOLUME_KEY    = "vol-%s-%s"

	POD_PREFIX           = "pod-"
	POD_CONTAINER_PREFIX = "pod-container-"
	POD_VOLUME_PREFIX    = "vol-%s"
	POD_VM_PREFIX        = "vm-"
)

//the id is a vm id
//and the content is vm data content
func keyVMData(id string) []byte {
	return []byte(fmt.Sprintf(VM_KEY, id))
}

// the id is a pod id
// and the db content is the vm id
func keyP2V(id string) []byte {
	return []byte(fmt.Sprintf(POD_VM_KEY, id))
}

// the id is a pod id
// and the db containt is the containers separated by comma
func keyP2C(id string) []byte {
	return []byte(fmt.Sprintf(POD_CONTAINER_KEY, id))
}

func keyPod(id string) []byte {
	return []byte(fmt.Sprintf(POD_KEY, id))
}

func keyVolume(pod string, volume string) []byte {
	return []byte(fmt.Sprintf(POD_VOLUME_KEY, pod, volume))
}

func prefixPod() []byte {
	return []byte(POD_PREFIX)
}

func prefixP2V() []byte {
	return []byte(POD_VM_PREFIX)
}

// the id is pod id
func prefixVolume(podId string) []byte {
	return []byte(fmt.Sprintf(POD_VOLUME_PREFIX, podId))
}
