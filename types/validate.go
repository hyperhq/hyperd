package types

import (
	"errors"
	"fmt"
	"net"
	"reflect"
	"regexp"
	"strings"

	"github.com/hyperhq/hyperd/utils"
)

func (pod *UserPod) Validate() error {

	var volume_drivers = map[string]bool{
		"raw":   true,
		"qcow2": true,
		"vdi":   true,
		"vfs":   true,
		"rbd":   true,
	}

	hostnameLen := len(pod.Hostname)
	if hostnameLen > 63 {
		return fmt.Errorf("Hostname exceeds the maximum length 63, len: %d", hostnameLen)
	}
	if hostnameLen > 0 {
		for _, seg := range strings.Split(pod.Hostname, ".") {
			if !utils.IsDNSLabel(seg) {
				return fmt.Errorf("Hostname should fullfil the pattern: %s, input hostname: %s", utils.Dns1123LabelFmt, pod.Hostname)
			}
		}
	}

	hasGw := false
	for idx, config := range pod.Interfaces {
		if config.Gateway == "" {
			continue
		}
		if hasGw {
			return fmt.Errorf("in interface %d, Other interface already configured Gateway", idx)
		}
		hasGw = true
	}

	uniq, vset := keySet(pod.Volumes)
	if !uniq {
		if len(vset) > 0 {
			return errors.New("Volumes name does not unique")
		}
	}

	uniq, fset := keySet(pod.Files)
	if !uniq {
		if len(fset) > 0 {
			return errors.New("Files name does not unique")
		}
	}
	var permReg = regexp.MustCompile("0[0-7]{3}")
	for idx, container := range pod.Containers {

		if uniq, _ := keySet(container.Volumes); !uniq {
			return fmt.Errorf("in container %d, volume source are not unique", idx)
		}

		if uniq, _ := keySet(container.Envs); !uniq {
			return fmt.Errorf("in container %d, environment name are not unique", idx)
		}

		for _, f := range container.Files {
			if _, ok := fset[f.Filename]; !ok {
				return fmt.Errorf("in container %d, file %s does not exist in file list.", idx, f.Filename)
			}
			if f.Perm == "" {
				f.Perm = "0755"
			}
			if f.Perm != "0" {
				if !permReg.Match([]byte(f.Perm)) {
					return fmt.Errorf("in container %d, the permission %s only accept Octal digital in string", idx, f.Perm)
				}
			}
		}

		for _, v := range container.Volumes {
			if _, ok := vset[v.Volume]; !ok {
				return fmt.Errorf("in container %d, volume %s does not exist in volume list.", idx, v.Volume)
			}
		}
	}

	for idx, v := range pod.Volumes {
		if v.Format == "" {
			continue
		}

		if _, ok := volume_drivers[v.Format]; !ok {
			return fmt.Errorf("in volume %d, volume does not support driver %s.", idx, v.Format)
		}
	}

	for _, dns := range pod.Dns {
		if ip := net.ParseIP(dns); ip == nil {
			return fmt.Errorf("incorrect dns %s.", dns)
		}
	}

	return nil
}

type item interface {
	key() string
}

func keySet(ilist interface{}) (bool, map[string]bool) {
	tmp, err := InterfaceSlice(ilist)
	if err != nil {
		return false, nil
	}
	iset := make(map[string]bool)
	for _, x := range tmp {
		switch x.(type) {
		case item:
			kx := x.(item).key()
			if _, ok := iset[kx]; ok {
				return false, iset
			}
			iset[kx] = true
			break
		default:
			return false, iset
		}
	}
	return true, iset
}

func (vol UserVolume) key() string          { return vol.Name }
func (vol UserVolumeReference) key() string { return vol.Volume }
func (f UserFile) key() string              { return f.Name }
func (env EnvironmentVar) key() string      { return env.Env }

func InterfaceSlice(slice interface{}) ([]interface{}, error) {
	s := reflect.ValueOf(slice)
	if s.Kind() != reflect.Slice {
		return nil, fmt.Errorf("InterfaceSlice() given a non-slice type")
	}

	ret := make([]interface{}, s.Len())

	for i := 0; i < s.Len(); i++ {
		ret[i] = s.Index(i).Interface()
	}

	return ret, nil
}
