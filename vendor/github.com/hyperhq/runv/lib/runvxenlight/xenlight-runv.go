// +build linux,with_xen490

/*
 *
 * This library is free software; you can redistribute it and/or
 * modify it under the terms of the GNU Lesser General Public
 * License as published by the Free Software Foundation;
 * version 2.1 of the License.
 *
 * This library is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the GNU
 * Lesser General Public License for more details.
 *
 * You should have received a copy of the GNU Lesser General Public
 * License along with this library; If not, see <http://www.gnu.org/licenses/>.
 */
package xenlight

/*
#cgo LDFLAGS: -lxenlight -lyajl -lxentoollog
#include <stdlib.h>
#include <libxl.h>
#include <libxl_utils.h>

typedef struct runvxl_domain_config {
        int             dom_type;
        int             domid;
        char*           name;
        char*           uuid;
        char*           kernel;
        char*           initrd;
        char*           cmdline;

        int             max_vcpus;
        uint64_t        max_memkb;

        char            *p9_tag;
        char            *p9_path;

        char            *hyper_path;
        char            *hyper_name;
        char            *tty_path;
        char            *tty_name;

        void            *extra;
} runvxl_domain_config;

static int runvxl_domain_create_new(libxl_ctx *ctx, runvxl_domain_config *config) {
    int i;
    uint32_t domid = 0;
    libxl_domain_config d_config;

    libxl_domain_config_init(&d_config);

    d_config.num_p9s = 1;
#ifndef LIBXL_HAVE_P9S
//this flag is introduce since tag 4.10.0-rc1, introduce a field rename
    d_config.p9 = malloc(sizeof(libxl_device_p9));
    if (d_config.p9 == NULL) {
        return -1;
    }
    d_config.p9->tag = config->p9_tag;
    d_config.p9->path = config->p9_path;
    d_config.p9->security_model = "none";
#else //LIBXL_HAVE_P9S
    d_config.p9s = malloc(sizeof(libxl_device_p9));
    if (d_config.p9s == NULL) {
        return -1;
    }
    d_config.p9s->tag = config->p9_tag;
    d_config.p9s->path = config->p9_path;
    d_config.p9s->security_model = "none";
#endif //LIBXL_HAVE_P9S


    d_config.num_channels = 2;
    d_config.channels = malloc(sizeof(libxl_device_channel) * 2);

    d_config.channels[0].name = config->hyper_name;
    d_config.channels[0].connection = LIBXL_CHANNEL_CONNECTION_SOCKET;
    d_config.channels[0].u.socket.path = config->hyper_path;

    d_config.channels[1].name = config->tty_name;
    d_config.channels[1].connection = LIBXL_CHANNEL_CONNECTION_SOCKET;
    d_config.channels[1].u.socket.path = config->tty_path;

    d_config.c_info.type = config->dom_type;

    d_config.b_info.type = config->dom_type;
    d_config.b_info.max_vcpus = config->max_vcpus;
    d_config.b_info.max_memkb = config->max_memkb;
    d_config.b_info.kernel = config->kernel;
    d_config.b_info.ramdisk = config->initrd;
    d_config.b_info.cmdline = config->cmdline;

    libxl_cpu_bitmap_alloc(ctx, &d_config.b_info.avail_vcpus, config->max_vcpus);
    libxl_bitmap_set_none(&d_config.b_info.avail_vcpus);
    for (i = 0; i < config->max_vcpus; i++)
        libxl_bitmap_set((&d_config.b_info.avail_vcpus), i);

    if (libxl_domain_create_new(ctx, &d_config, &domid, 0, 0))
        return -1;

    return domid;
}

static int runvxl_domain_create_new_from_json(libxl_ctx *ctx, char *config) {
    int ret;
    uint32_t domid = 0;
    libxl_domain_config d_config;

    libxl_domain_config_from_json(ctx, &d_config, config);

    if (libxl_domain_create_new(ctx, &d_config, &domid, 0, 0))
        return -1;

    return domid;
}

static void runvxl_sigchld_handler(libxl_ctx* ctx) {
    int status, res;
    pid_t pid = waitpid(-1, &status, WNOHANG);
    printf("got child pid: %d\n", pid);
    if (pid > 0) {
        res = libxl_childproc_reaped(ctx, pid, status);
        printf("check whether child proc is created by libxl: %d\n", res);
    }
}

static const libxl_childproc_hooks childproc_hooks = {
    .chldowner = libxl_sigchld_owner_mainloop,
};

void runvxl_childproc_setmode(libxl_ctx *ctx) {
    libxl_childproc_setmode(ctx, &childproc_hooks, 0);
}

int runvxl_add_nic(libxl_ctx *ctx, int32_t domid, int id, char *bridge, char *device, char *mac) {
    libxl_device_nic nic;
    int ret;

    libxl_device_nic_init(&nic);
    nic.bridge = strdup(bridge);
    nic.nictype = LIBXL_NIC_TYPE_VIF;
    nic.devid = id;
    nic.ifname = strdup(device);
    // TODO: why self defined mac cause nic fail to up?
    //memcpy(nic.mac, mac, 6);

    ret = libxl_device_nic_add(ctx, domid, &nic, 0);
    libxl_device_nic_dispose(&nic);
    return ret;
}

int runvxl_remove_nic(libxl_ctx *ctx, int32_t domid, int id) {
    libxl_device_nic nic;
    int ret;

    libxl_device_nic_init(&nic);
    ret = libxl_devid_to_device_nic(ctx, domid, id, &nic);
    if (ret)
        goto cleanup;
    ret = libxl_device_nic_remove(ctx, domid, &nic, 0);
cleanup:
    libxl_device_nic_dispose(&nic);
    return ret;
}

int runvxl_add_disk(libxl_ctx *ctx, int32_t domid, char *filename, char *vdev, bool readwrite) {
    libxl_device_disk disk;
    int ret;

    libxl_device_disk_init(&disk);
    disk.format = LIBXL_DISK_FORMAT_RAW;
    disk.pdev_path = strdup(filename);
    disk.vdev = strdup(vdev);
    disk.readwrite = readwrite;

    ret = libxl_device_disk_add(ctx, domid, &disk, 0);
    libxl_device_disk_dispose(&disk);
    return ret;
}

int runvxl_remove_disk(libxl_ctx *ctx, int32_t domid, char *vdev) {
    libxl_device_disk disk;
    int ret;

    libxl_device_disk_init(&disk);
    ret = libxl_vdev_to_device_disk(ctx, domid, vdev, &disk);
    if (ret)
        goto cleanup;
    ret = libxl_device_disk_remove(ctx, domid, &disk, 0);
cleanup:
    libxl_device_disk_dispose(&disk);
    return ret;
}


int runvxl_domain_destroy_byname(libxl_ctx *ctx, char *name) {
    int ret;
    uint32_t domid;

    ret = libxl_domain_qualifier_to_domid(ctx, name, &domid);
    if (ret){
        return ret;
    }

    return libxl_domain_destroy(ctx, domid, 0);
}
*/
import "C"

/*
 * Other flags that may be needed at some point:
 *  -lnl-route-3 -lnl-3
 *
 * To get back to static linking:
 * #cgo LDFLAGS: -lxenlight -lyajl_s -lxengnttab -lxenstore -lxenguest -lxentoollog -lxenevtchn -lxenctrl -lblktapctl -lxenforeignmemory -lxencall -lz -luuid -lutil
 */

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"unsafe"
)

type CreateInfo struct {
	Type string `json:"type"`
	Name string `json:"name"`
	Uuid string `json:"uuid"`
}

type PvInfo struct {
	SlackMemory    int    `json:"slack_memkb,omitempty"`
	Bootloader     string `json:"bootloader,omitempty"`
	BootloaderArgs string `json:"bootloader_args,omitempty"`
}

type BuildInfo struct {
	MaxVcpus          int    `json:"max_vcpus"`
	MaxMemory         uint64 `json:"max_memkb"`
	TargetMemory      uint64 `json:"target_memkb"`
	ClaimMode         string `json:"claim_mode"`
	DeviceModeVersion string `json:"device_model_version"`
	Kernel            string `json:"kernel"`
	Initrd            string `json:"ramdisk"`
	Cmdline           string `json:"cmdline"`
	PvInfo            PvInfo `json:"type.pv"`
}

type P9Info struct {
	ShareTag      string `json:"tag"`
	ShareDir      string `json:"path"`
	SecurityModel string `json:"security_model"`
}

type SocketInfo struct {
	Path string `json:"path"`
}

type ChannelInfo struct {
	DevId  int        `json:"devid"`
	Name   string     `json:"name"`
	Socket SocketInfo `json:"connection.socket"`
}

type DomainConfig struct {
	Cinfo CreateInfo `json:"c_info"`
	Binfo BuildInfo  `json:"b_info"`
	//Disks    []DiskInfo    `json:"disks"`
	Channels []ChannelInfo `json:"channels"`
	P9       []P9Info      `json:"p9"`

	domType int
}

func (dc *DomainConfig) toC() (rdc C.struct_runvxl_domain_config) {
	rdc.dom_type = C.int(dc.domType)

	cinfo := dc.Cinfo
	rdc.uuid = C.CString(cinfo.Uuid)
	rdc.name = C.CString(cinfo.Name)

	binfo := dc.Binfo
	rdc.kernel = C.CString(binfo.Kernel)
	rdc.initrd = C.CString(binfo.Initrd)
	rdc.cmdline = C.CString(binfo.Cmdline)

	rdc.max_memkb = C.uint64_t(binfo.MaxMemory)
	rdc.max_vcpus = C.int(binfo.MaxVcpus)

	p9 := dc.P9[0]
	rdc.p9_tag = C.CString(p9.ShareTag)
	rdc.p9_path = C.CString(p9.ShareDir)

	rdc.hyper_path = C.CString(dc.Channels[1].Socket.Path)
	rdc.hyper_name = C.CString(dc.Channels[1].Name)

	rdc.tty_path = C.CString(dc.Channels[2].Socket.Path)
	rdc.tty_name = C.CString(dc.Channels[2].Name)

	return
}

func (Ctx *Context) CreateNewDomain(config *DomainConfig) (int, error) {
	if err := Ctx.CheckOpen(); err != nil {
		return -1, err
	}
	rdc := config.toC()
	ret := C.runvxl_domain_create_new(Ctx.ctx, &rdc)
	if ret < 0 {
		return -1, fmt.Errorf("fail to create xen domain: %d", ret)
	}

	return int(ret), nil
}

func (Ctx *Context) DestroyDomain(domId Domid) error {
	ret := C.libxl_domain_destroy(Ctx.ctx, C.uint32_t(domId), (*C.libxl_asyncop_how)(nil))
	if ret != 0 {
		return fmt.Errorf("fail to destroy dom %v: %v\n", domId, ret)
	}
	return nil
}

func (Ctx *Context) DestroyDomainByName(name string) error {
	ret := C.runvxl_domain_destroy_byname(Ctx.ctx, C.CString(name))
	if ret != 0 {
		return fmt.Errorf("fail to destroy dom %v: %v\n", name, ret)
	}
	return nil
}

func (Ctx *Context) CreateNewDomainFromJson(config string) (int, error) {
	if err := Ctx.CheckOpen(); err != nil {
		return -1, err
	}
	ret := C.runvxl_domain_create_new_from_json(Ctx.ctx, C.CString(config))
	if ret < 0 {
		return -1, fmt.Errorf("fail to create xen domain: %d", ret)
	}

	return int(ret), nil
}

func (Ctx *Context) SigChildHandle() {
	sigchan := make(chan os.Signal, 1)
	go func() {
		for {
			_, ok := <-sigchan
			if !ok {
				break
			}
			C.runvxl_sigchld_handler(Ctx.ctx)
		}
	}()
	signal.Notify(sigchan, syscall.SIGCHLD)

	C.runvxl_childproc_setmode(Ctx.ctx)
}

func (Ctx *Context) DomainAddNic(domId Domid, id int, bridge, device, mac string) error {
	if ret := C.runvxl_add_nic(Ctx.ctx, C.int32_t(domId), C.int(id), C.CString(bridge), C.CString(device), C.CString(mac)); ret != 0 {
		return fmt.Errorf("fail to add nic %v for dom %v", id, domId)
	}
	return nil
}

func (Ctx *Context) DomainRemoveNic(domId Domid, id int) error {
	if ret := C.runvxl_remove_nic(Ctx.ctx, C.int32_t(domId), C.int(id)); ret != 0 {
		return fmt.Errorf("fail to remove nic %v for dom %v", id, domId)
	}
	return nil
}

func (Ctx *Context) DomainAddDisk(domId Domid, filename, vdev string, readwrite bool) error {
	if ret := C.runvxl_add_disk(Ctx.ctx, C.int32_t(domId), C.CString(filename), C.CString(vdev), C.bool(readwrite)); ret != 0 {
		return fmt.Errorf("fail to add vdev %v %v from dom %v\n", vdev, filename, domId)
	}
	return nil
}

func (Ctx *Context) DomainRemoveDisk(domId Domid, vdev string) error {
	if ret := C.runvxl_remove_disk(Ctx.ctx, C.int32_t(domId), C.CString(vdev)); ret != 0 {
		return fmt.Errorf("fail to remove vdev %v from dom %v\n", vdev, domId)
	}
	return nil
}

func (Ctx *Context) DomainQualifierToId(name string) (Domid, error) {
	var id Domid
	if ret := C.libxl_domain_qualifier_to_domid(Ctx.ctx, C.CString(name), (*C.uint32_t)(unsafe.Pointer(&id))); ret != 0 {
		return 0, fmt.Errorf("fail to get id for domain %v: v", name, ret)
	}
	return id, nil
}

func GenerateUuid() (string, error) {
	var buf [16]byte
	var uuid [36]byte

	_, err := io.ReadFull(rand.Reader, buf[:])
	if err != nil {
		return "", err
	}
	buf[6] = (buf[6] & 0x0f) | 0x40 // Version 4
	buf[8] = (buf[8] & 0x3f) | 0x80 // Variant is 10

	hex.Encode(uuid[:], buf[:4])
	uuid[8] = '-'
	hex.Encode(uuid[9:13], buf[4:6])
	uuid[13] = '-'
	hex.Encode(uuid[14:18], buf[6:8])
	uuid[18] = '-'
	hex.Encode(uuid[19:23], buf[8:10])
	uuid[23] = '-'
	hex.Encode(uuid[24:], buf[10:])

	return string(uuid[:]), nil
}
