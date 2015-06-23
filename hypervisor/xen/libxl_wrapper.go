// +build linux

package xen

/*
#include "hyperxl.h"

#cgo LDFLAGS: -L.

void DomainDeath_cgo(uint32_t domid);
*/
import "C"

import (
	"hyper/lib/glog"
	"unsafe"
	"hyper/hypervisor"
)

type (
	XentoollogLogger C.struct_xentoollog_logger

	LibxlCtxPtr *C.struct_libxl__ctx

	LibxlEvent C.struct_libxl_event

	LibxlDiskBackend C.enum_libxl_disk_backend
	LibxlDiskFormat  C.enum_libxl_disk_format

	HyperxlDomainConfig C.struct_hyperxl_domain_config
	HyperxlDiskConfig   C.struct_hyperxl_disk_config
	HyperxlNicConfig    C.struct_hyperxl_nic_config
)

const (
	LIBXL_DISK_FORMAT_UNKNOWN = C.LIBXL_DISK_FORMAT_UNKNOWN
	LIBXL_DISK_FORMAT_QCOW    = C.LIBXL_DISK_FORMAT_QCOW
	LIBXL_DISK_FORMAT_QCOW2   = C.LIBXL_DISK_FORMAT_QCOW2
	LIBXL_DISK_FORMAT_VHD     = C.LIBXL_DISK_FORMAT_VHD
	LIBXL_DISK_FORMAT_RAW     = C.LIBXL_DISK_FORMAT_RAW
	LIBXL_DISK_FORMAT_EMPTY   = C.LIBXL_DISK_FORMAT_EMPTY

	LIBXL_DISK_BACKEND_UNKNOWN = C.LIBXL_DISK_BACKEND_UNKNOWN
	LIBXL_DISK_BACKEND_PHY     = C.LIBXL_DISK_BACKEND_PHY
	LIBXL_DISK_BACKEND_TAP     = C.LIBXL_DISK_BACKEND_TAP
	LIBXL_DISK_BACKEND_QDISK   = C.LIBXL_DISK_BACKEND_QDISK
)

func (dc *DomainConfig) toC() *C.struct_hyperxl_domain_config {
	l := len(dc.Extra)
	extra := make([]unsafe.Pointer, l+1)
	for i := 0; i < l; i++ {
		extra[i] = unsafe.Pointer(C.CString(dc.Extra[i]))
	}
	extra[l] = unsafe.Pointer(nil)

	return &C.struct_hyperxl_domain_config{
		hvm:           (C.bool)(dc.Hvm),
		domid:         0,
		name:          C.CString(dc.Name),
		kernel:        C.CString(dc.Kernel),
		initrd:        C.CString(dc.Initrd),
		cmdline:       C.CString(dc.Cmdline),
		max_vcpus:     (C.int)(dc.MaxVcpus),
		max_memory_kb: (C.int)(dc.MaxMemory),
		console_sock:  C.CString(dc.ConsoleSock),
		extra:         unsafe.Pointer(&extra),
	}
}

//int  hyperxl_initialize_driver(hyperxl_driver** pdriver);
func HyperxlInitializeDriver() (*XenDriver, int) {

	var driver *C.struct_hyperxl_driver = nil
	res := (int)(C.hyperxl_initialize_driver(&driver))
	if res == 0 {
		defer C.free(unsafe.Pointer(driver))

		return &XenDriver{
			Ctx:          driver.ctx,
			Version:      (uint32)(driver.version),
			Capabilities: C.GoString(driver.capabilities),
			Logger:       (*XentoollogLogger)(driver.logger),
		}, 0
	}
	return nil, res
}

//int  hyperxl_domain_start(libxl_ctx* ctx, hyperxl_domain_config* config);
func HyperxlDomainStart(ctx LibxlCtxPtr, config *DomainConfig) (int, int) {
	cc := config.toC()
	res := (int)(C.hyperxl_domain_start((*C.struct_libxl__ctx)(ctx), cc))
	return (int)(cc.domid), res
}

//void hyperxl_sigchld_handler(libxl_ctx* ctx)
func HyperxlSigchldHandler(ctx LibxlCtxPtr) {
	C.hyperxl_sigchld_handler((*C.struct_libxl__ctx)(ctx))
}

//int  hyperxl_domain_destroy(libxl_ctx* ctx, uint32_t domid)
func HyperxlDomainDestroy(ctx LibxlCtxPtr, domid uint32) int {
	return (int)(C.hyperxl_domain_destroy((*C.struct_libxl__ctx)(ctx), (C.uint32_t)(domid)))
}

func HyperxlDomainCheck(ctx LibxlCtxPtr, domid uint32) int {
	return (int)(C.hyperxl_domaim_check((*C.struct_libxl__ctx)(ctx), (C.uint32_t)(domid)))
}

//int hyperxl_nic_add(libxl_ctx* ctx, uint32_t domid, hyperxl_nic_config* config);
func HyperxlNicAdd(ctx LibxlCtxPtr, domid uint32, ip, bridge, gatewaydev, ifname string, mac []byte) int {
	var nic *HyperxlNicConfig = &HyperxlNicConfig{
		ip:         C.CString(ip),
		bridge:     C.CString(bridge),
		gatewaydev: C.CString(gatewaydev),
		mac:        (*C.uint8_t)(&mac[0]),
		ifname:     C.CString(ifname),
	}
	return (int)(C.hyperxl_nic_add((*C.struct_libxl__ctx)(ctx), (C.uint32_t)(domid), (*C.struct_hyperxl_nic_config)(nic)))
}

//int hyperxl_nic_remove(libxl_ctx* ctx, uint32_t domid, const char* mac);
func HyperxlNicRemove(ctx LibxlCtxPtr, domid uint32, mac string) int {
	return (int)(C.hyperxl_nic_remove((*C.struct_libxl__ctx)(ctx), (C.uint32_t)(domid), C.CString(mac)))
}

//int hyperxl_disk_add(libxl_ctx* ctx, uint32_t domid,hyperxl_disk_config* config);
func HyperxlDiskAdd(ctx LibxlCtxPtr, domid uint32, source, target string, backend LibxlDiskBackend, format LibxlDiskFormat) int {
	var disk *HyperxlDiskConfig = &HyperxlDiskConfig{
		source:  C.CString(source),
		target:  C.CString(target),
		backend: (C.libxl_disk_backend)(backend),
		format:  (C.libxl_disk_format)(format),
	}
	return (int)(C.hyperxl_disk_add((*C.struct_libxl__ctx)(ctx), (C.uint32_t)(domid), (*C.struct_hyperxl_disk_config)(disk)))
}

//int hyperxl_disk_remove(libxl_ctx* ctx, uint32_t domid,hyperxl_disk_config* config);
func HyperxlDiskRemove(ctx LibxlCtxPtr, domid uint32, source, target string, backend LibxlDiskBackend, format LibxlDiskFormat) int {
	var disk *HyperxlDiskConfig = &HyperxlDiskConfig{
		source:  C.CString(source),
		target:  C.CString(target),
		backend: (C.libxl_disk_backend)(backend),
		format:  (C.libxl_disk_format)(format),
	}
	return (int)(C.hyperxl_disk_remove((*C.struct_libxl__ctx)(ctx), (C.uint32_t)(domid), (*C.struct_hyperxl_disk_config)(disk)))
}

/******************************************************************************
 *  Libxl directly
 ******************************************************************************/

//int libxl_ctx_free(libxl_ctx *ctx /* 0 is OK */);
func LibxlCtxFree(ctx LibxlCtxPtr) int {
	return (int)(C.libxl_ctx_free((*C.struct_libxl__ctx)(ctx)))
}

/******************************************************************************
 *  Callbacks
 ******************************************************************************/

//export DomainDeath_cgo
func DomainDeath_cgo(domid C.uint32_t) {
	defer func(){ recover() }() //in case the vmContext or channel has been released
	dom := (uint32)(domid)
	glog.Infof("got xen hypervisor message: domain %d quit")
	if vm,ok := globalDriver.domains[dom]; ok {
		glog.V(1).Infof("Domain %d managed by xen driver, try close it")
		delete(globalDriver.domains, dom)
		vm.Hub <- &hypervisor.VmExit{}
	}
}

//export hyperxl_log_cgo
func hyperxl_log_cgo(msg *C.char, len C.int) {
	if glog.V(1) {
		glog.Info("[libxl] ", C.GoStringN(msg, len))
	}
}

/*
//libxl.h:int libxl_domain_create_restore(libxl_ctx *ctx, libxl_domain_config *d_config,
//libxl.h-                                uint32_t *domid, int restore_fd,
//libxl.h-                                const libxl_domain_restore_params *params,
//libxl.h-                                const libxl_asyncop_how *ao_how,
//libxl.h-                                const libxl_asyncprogress_how *aop_console_how)
func LibxlDomainCreateRestore(ctx LibxlCtxPtr, d_config *LibxlDomainConfig, domid *uint32, restore_fd int,
                              params *LibxlDomainRestoreParams,
                              ao_how *LibxlAsyncopHow, aop_console_how *LibxlAsyncprogressHow) int {
    return (int)(C.libxl_domain_create_restore((*C.struct_libxl__ctx)(ctx), (*C.struct_libxl_domain_config)(d_config),
                                               (*C.uint32_t)(domid), (C.int)(restore_fd),
                                               (*C.struct_libxl_domain_restore_params)(params),
                                               (*C.struct___5)(ao_how),
                                               (*C.struct___9)(aop_console_how)))
}


//libxl_event.h:int libxl_evenable_domain_death(libxl_ctx *ctx, uint32_t domid,
//libxl_event.h-                         libxl_ev_user, libxl_evgen_domain_death **evgen_out);
func LibxlEvenableDomainDeath(ctx LibxlCtxPtr, domid uint32, user uint64) (*LibxlEvgenDomainDeath,int) {
    var event *C.struct_libxl__evgen_domain_death = (*C.struct_libxl__evgen_domain_death)(nil)
    res := C.libxl_evenable_domain_death((*C.struct_libxl__ctx)(ctx), (C.uint32_t)(domid), (C.libxl_ev_user)(user), &event)
    if (int)(res) != 0 {
        return nil, (int)(res)
    }
    return (*LibxlEvgenDomainDeath)(event),0
}

//void libxl_evdisable_domain_death(libxl_ctx *ctx, libxl_evgen_domain_death*);
func LibxlEvdisableDomainDeath(ctx LibxlCtxPtr, event *LibxlEvgenDomainDeath) {
    C.libxl_evdisable_domain_death((*C.struct_libxl__ctx)(ctx), (*C.struct_libxl__evgen_domain_death)(event))
}

*/
