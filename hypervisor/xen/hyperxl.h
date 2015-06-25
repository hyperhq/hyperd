#ifndef __HYPER_LIBXL_CONNECTOR__
#define __HYPER_LIBXL_CONNECTOR__

#include "config.h"

#ifdef WITH_XEN

#include <libxl.h>
#include <libxl_utils.h>

#else // defines xen types & methods here if no xen header available

#include <stdbool.h>
#include <stdint.h>

typedef struct xentoollog_logger{
} xentoollog_logger;

typedef struct libxl__ctx{
} libxl_ctx;

typedef struct libxl_event{
} libxl_event;

typedef enum libxl_disk_format {
    LIBXL_DISK_FORMAT_UNKNOWN = 0,
    LIBXL_DISK_FORMAT_QCOW = 1,
    LIBXL_DISK_FORMAT_QCOW2 = 2,
    LIBXL_DISK_FORMAT_VHD = 3,
    LIBXL_DISK_FORMAT_RAW = 4,
    LIBXL_DISK_FORMAT_EMPTY = 5,
} libxl_disk_format;

typedef enum libxl_disk_backend {
    LIBXL_DISK_BACKEND_UNKNOWN = 0,
    LIBXL_DISK_BACKEND_PHY = 1,
    LIBXL_DISK_BACKEND_TAP = 2,
    LIBXL_DISK_BACKEND_QDISK = 3,
} libxl_disk_backend;

int libxl_ctx_free(libxl_ctx *ctx /* 0 is OK */);
int libxl_event_free(libxl_ctx *ctx, libxl_event *event);

#endif //WITH_XEN

#include <stdlib.h>
#include <stdio.h>
#include <string.h>
#include <sys/types.h>
#include <sys/wait.h>

#ifndef LIBXL_HAVE_NONCONST_EVENT_OCCURS_EVENT_ARG
  #define HYPERXL_EVENT_CONST const
#else
  #define HYPERXL_EVENT_CONST
#endif

typedef struct hyperxl_driver {
	libxl_ctx*			ctx;
	uint32_t 			version;
	char*               capabilities;
	xentoollog_logger* 	logger;
} hyperxl_driver;

typedef struct hyperxl_domain_config {
	bool 	hvm;

    int     domid;
    void*   ev;
	const char* name;

	const char* kernel;
	const char* initrd;
	const char* cmdline;

	int 	max_vcpus;
	int 	max_memory_kb;

	const char* console_sock;
	void*   extra;
} hyperxl_domain_config;

//data structure defined in libxl_types.idl:
//libxl_device_disk = Struct("device_disk", [
//    ("backend_domid", libxl_domid),
//    ("backend_domname", string),
//    ("pdev_path", string),
//    ("vdev", string),
//    ("backend", libxl_disk_backend),
//    ("format", libxl_disk_format),
//    ("script", string),
//    ("removable", integer),
//    ("readwrite", integer),
//    ("is_cdrom", integer),
//    ("direct_io_safe", bool),
//    ("discard_enable", libxl_defbool),
//    ])
typedef struct hyperxl_disk_config {
    const char* source;
    const char* target;
    libxl_disk_backend backend;
    libxl_disk_format  format;
} hyperxl_disk_config;

//data structure defined in libxl_types.idl:
//libxl_device_nic = Struct("device_nic", [
//    ("backend_domid", libxl_domid),
//    ("backend_domname", string),
//    ("devid", libxl_devid),
//    ("mtu", integer),
//    ("model", string),
//    ("mac", libxl_mac),
//    ("ip", string),
//    ("bridge", string),
//    ("ifname", string),
//    ("script", string),
//    ("nictype", libxl_nic_type),
//    ("rate_bytes_per_interval", uint64),
//    ("rate_interval_usecs", uint32),
//    ("gatewaydev", string),
//    ])
typedef struct hyperxl_nic_config {
    const char* ip;
    const char* bridge;
    const char* gatewaydev;
    uint8_t*    mac;
    const char* ifname;
} hyperxl_nic_config;

int  hyperxl_initialize_driver(hyperxl_driver** pdriver);

void hyperxl_destroy_driver(hyperxl_driver* driver);

int  hyperxl_domain_start(libxl_ctx* ctx, hyperxl_domain_config* config);

void hyperxl_domain_cleanup(libxl_ctx *ctx, void* ev);

int  hyperxl_domain_destroy(libxl_ctx* ctx, uint32_t domid);

int  hyperxl_domaim_check(libxl_ctx* ctx, uint32_t domid);

void hyperxl_sigchld_handler(libxl_ctx* ctx);

void hyperxl_domain_event_handler(void *data, HYPERXL_EVENT_CONST libxl_event *event);

extern void DomainDeath_cgo(uint32_t domid);
extern void hyperxl_log_cgo(char* buf, int len);

int hyperxl_nic_add(libxl_ctx* ctx, uint32_t domid, hyperxl_nic_config* config);
int hyperxl_nic_remove(libxl_ctx* ctx, uint32_t domid, const char* mac);

int hyperxl_disk_add(libxl_ctx* ctx, uint32_t domid,hyperxl_disk_config* config);
int hyperxl_disk_remove(libxl_ctx* ctx, uint32_t domid,hyperxl_disk_config* config);

#endif//__HYPER_LIBXL_CONNECTOR__
