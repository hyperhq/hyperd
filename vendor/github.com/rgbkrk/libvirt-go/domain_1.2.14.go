// +build libvirt.1.2.14

package libvirt

/*
#cgo LDFLAGS: -lvirt-qemu -lvirt
#include <libvirt/libvirt.h>
#include <libvirt/libvirt-qemu.h>
#include <libvirt/virterror.h>
#include <stdlib.h>
*/
import "C"

import (
	"errors"
	"reflect"
	"unsafe"
)

type VirDomainIPAddress struct {
	Type   int
	Addr   string
	Prefix uint
}

type VirDomainInterface struct {
	Name   string
	Hwaddr string
	Addrs  []VirDomainIPAddress
}

func (d *VirDomain) ListAllInterfaceAddresses(src uint) ([]VirDomainInterface, error) {
	var cList *C.virDomainInterfacePtr
	numIfaces := int(C.virDomainInterfaceAddresses(d.ptr, (**C.virDomainInterfacePtr)(&cList), C.uint(src), 0))
	if numIfaces == -1 {
		return nil, GetLastError()
	}

	hdr := reflect.SliceHeader{
		Data: uintptr(unsafe.Pointer(cList)),
		Len:  int(numIfaces),
		Cap:  int(numIfaces),
	}

	ifaces := make([]VirDomainInterface, numIfaces)
	ifaceSlice := *(*[]C.virDomainInterfacePtr)(unsafe.Pointer(&hdr))

	for i := 0; i < numIfaces; i++ {
		ifaces[i].Name = C.GoString(ifaceSlice[i].name)
		ifaces[i].Hwaddr = C.GoString(ifaceSlice[i].hwaddr)

		numAddr := int(ifaceSlice[i].naddrs)
		addrHdr := reflect.SliceHeader{
			Data: uintptr(unsafe.Pointer(&ifaceSlice[i].addrs)),
			Len:  int(numAddr),
			Cap:  int(numAddr),
		}

		ifaces[i].Addrs = make([]VirDomainIPAddress, numAddr)
		addrSlice := *(*[]C.virDomainIPAddressPtr)(unsafe.Pointer(&addrHdr))

		for k := 0; k < numAddr; k++ {
			ifaces[i].Addrs[k] = VirDomainIPAddress{}
			ifaces[i].Addrs[k].Type = int(addrSlice[k]._type)
			ifaces[i].Addrs[k].Addr = C.GoString(addrSlice[k].addr)
			ifaces[i].Addrs[k].Prefix = uint(addrSlice[k].prefix)

		}
		C.virDomainInterfaceFree(ifaceSlice[i])
	}
	C.free(unsafe.Pointer(cList))
	return ifaces, nil
}

func (dest *VirTypedParameters) loadToCPtr() (params C.virTypedParameterPtr, nParams C.int, err error) {

	var maxParams C.int = 0
	params = nil

	for _, param := range *dest {

		switch param.Name {
		case VIR_DOMAIN_BLOCK_COPY_GRANULARITY:
			value, ok := param.Value.(uint)
			if !ok {
				err = errors.New("Mismatched parameter value type")
				return
			}

			cName := C.CString(param.Name)
			defer C.free(unsafe.Pointer(cName))

			result := C.virTypedParamsAddUInt(&params, &nParams, &maxParams, cName, C.uint(value))
			if result == -1 {
				err = GetLastError()
				return
			}
		case VIR_DOMAIN_BLOCK_COPY_BANDWIDTH, VIR_DOMAIN_BLOCK_COPY_BUF_SIZE:
			value, ok := param.Value.(uint64)
			if !ok {
				err = errors.New("Mismatched parameter value type")
				return
			}

			cName := C.CString(param.Name)
			defer C.free(unsafe.Pointer(cName))

			result := C.virTypedParamsAddULLong(&params, &nParams, &maxParams, cName, C.ulonglong(value))
			if result == -1 {
				err = GetLastError()
				return
			}
		default:
			err = errors.New("Unknown parameter name: " + param.Name)
			return
		}
	}
	return
}

func (d *VirDomain) BlockCopy(disk string, destXML string, params VirTypedParameters, flags uint32) error {

	cDisk := C.CString(disk)
	defer C.free(unsafe.Pointer(cDisk))
	cDestXML := C.CString(destXML)
	defer C.free(unsafe.Pointer(cDestXML))
	if cParams, cnParams, err := params.loadToCPtr(); err != nil {
		C.virTypedParamsFree(cParams, cnParams)
		return err
	} else {
		defer C.virTypedParamsFree(cParams, cnParams)

		result := int(C.virDomainBlockCopy(d.ptr, cDisk, cDestXML, cParams, cnParams, C.uint(flags)))
		if result == -1 {
			return GetLastError()
		}
	}
	return nil
}
