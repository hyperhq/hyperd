// +build linux,with_xen

package xen

/*
#cgo LDFLAGS: -ldl

#include <dlfcn.h>

#include <libxl.h>
#include <libxl_utils.h>

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

typedef struct hyperxl_disk_config {
    const char* source;
    const char* target;
    libxl_disk_backend backend;
    libxl_disk_format  format;
} hyperxl_disk_config;

typedef struct hyperxl_nic_config {
    const char* ip;
    const char* bridge;
    const char* gatewaydev;
    char*    mac;
    const char* ifname;
} hyperxl_nic_config;

static inline int  hyperxl_initialize_driver(hyperxl_driver** pdriver, bool verbose);

static inline void hyperxl_destroy_driver(hyperxl_driver* driver);

static inline int hyperxl_ctx_free(libxl_ctx* ctx);

static inline int  hyperxl_domain_start(libxl_ctx* ctx, hyperxl_domain_config* config);

static inline void hyperxl_domain_cleanup(libxl_ctx *ctx, void* ev);

static inline int  hyperxl_domain_destroy(libxl_ctx* ctx, uint32_t domid);

static inline int  hyperxl_domaim_check(libxl_ctx* ctx, uint32_t domid);

static inline void hyperxl_sigchld_handler(libxl_ctx* ctx);

static inline void hyperxl_domain_event_handler(void *data, HYPERXL_EVENT_CONST libxl_event *event);

extern void DomainDeath_cgo(uint32_t domid);

extern void hyperxl_log_cgo(char* buf, int len);

static inline int hyperxl_nic_add(libxl_ctx* ctx, uint32_t domid, hyperxl_nic_config* config);
static inline int hyperxl_nic_remove(libxl_ctx* ctx, uint32_t domid, const char* mac);

static inline int hyperxl_disk_add(libxl_ctx* ctx, uint32_t domid,hyperxl_disk_config* config);
static inline int hyperxl_disk_remove(libxl_ctx* ctx, uint32_t domid,hyperxl_disk_config* config);

static const libxl_childproc_hooks libxl_child_hooks = {
    .chldowner = libxl_sigchld_owner_mainloop,
};

static const struct libxl_event_hooks ev_hooks = {
    .event_occurs_mask = LIBXL_EVENTMASK_ALL,
    .event_occurs = hyperxl_domain_event_handler,
    .disaster = NULL, //disaster means the libxl has some issue, rather than a domain
};

typedef struct xentoollog_logger_hyperxl xentoollog_logger_hyperxl;
static inline xentoollog_logger_hyperxl* xtl_createlogger_hyperxl(xentoollog_level min_level, unsigned flags);

typedef struct hyperxl_xen_fn {
  int (*libxl_bitmap_count_set)(const libxl_bitmap *cpumap);
  void (*libxl_bitmap_set)(libxl_bitmap *bitmap, int bit);
  int (*libxl_childproc_reaped)(libxl_ctx *ctx, pid_t, int status)
                             LIBXL_EXTERNAL_CALLERS_ONLY;
  void (*libxl_childproc_setmode)(libxl_ctx *ctx, const libxl_childproc_hooks *hooks,
                               void *user);
  int (*libxl_cpu_bitmap_alloc)(libxl_ctx *ctx, libxl_bitmap *cpumap, int max_cpus);
  int (*libxl_ctx_alloc)(libxl_ctx **pctx, int version,
                      unsigned flags,
                      xentoollog_logger *lg);
  int (*libxl_ctx_free)(libxl_ctx *ctx);
  void (*libxl_defbool_set)(libxl_defbool *db, bool b);
  int (*libxl_device_disk_add)(libxl_ctx *ctx, uint32_t domid,
                            libxl_device_disk *disk,
                            const libxl_asyncop_how *ao_how)
                            LIBXL_EXTERNAL_CALLERS_ONLY;
  void (*libxl_device_disk_dispose)(libxl_device_disk *p);
  void (*libxl_device_disk_init)(libxl_device_disk *p);
  int (*libxl_device_disk_remove)(libxl_ctx *ctx, uint32_t domid,
                               libxl_device_disk *disk,
                               const libxl_asyncop_how *ao_how)
                               LIBXL_EXTERNAL_CALLERS_ONLY;
  int (*libxl_device_nic_add)(libxl_ctx *ctx, uint32_t domid, libxl_device_nic *nic,
                           const libxl_asyncop_how *ao_how)
                           LIBXL_EXTERNAL_CALLERS_ONLY;
  void (*libxl_device_nic_dispose)(libxl_device_nic *p);
  void (*libxl_device_nic_init)(libxl_device_nic *p);
  int (*libxl_device_nic_remove)(libxl_ctx *ctx, uint32_t domid,
                              libxl_device_nic *nic,
                              const libxl_asyncop_how *ao_how)
                              LIBXL_EXTERNAL_CALLERS_ONLY;
  void (*libxl_domain_build_info_init_type)(libxl_domain_build_info *p, libxl_domain_type type);
  void (*libxl_domain_config_dispose)(libxl_domain_config *d_config);
  void (*libxl_domain_config_init)(libxl_domain_config *d_config);
  void (*libxl_domain_create_info_init)(libxl_domain_create_info *p);
  int (*libxl_domain_create_new)(libxl_ctx *ctx, libxl_domain_config *d_config,
                              uint32_t *domid,
                              const libxl_asyncop_how *ao_how,
                              const libxl_asyncprogress_how *aop_console_how)
                              LIBXL_EXTERNAL_CALLERS_ONLY;
  int (*libxl_domain_destroy)(libxl_ctx *ctx, uint32_t domid,
                           const libxl_asyncop_how *ao_how)
                           LIBXL_EXTERNAL_CALLERS_ONLY;
  int (*libxl_domain_info)(libxl_ctx*, libxl_dominfo *info_r,
                        uint32_t domid);
  int (*libxl_domain_unpause)(libxl_ctx *ctx, uint32_t domid);
  void (*libxl_evdisable_domain_death)(libxl_ctx *ctx, libxl_evgen_domain_death*);
  int (*libxl_evenable_domain_death)(libxl_ctx *ctx, uint32_t domid,
                           libxl_ev_user, libxl_evgen_domain_death **evgen_out);
  void (*libxl_event_free)(libxl_ctx *ctx, libxl_event *event);
  void (*libxl_event_register_callbacks)(libxl_ctx *ctx,
                                      const libxl_event_hooks *hooks, void *user);
  int (*libxl_get_free_memory)(libxl_ctx *ctx, uint32_t *memkb);
  unsigned long (*libxl_get_required_shadow_memory)(unsigned long maxmem_kb, unsigned int smp_cpus);
  const libxl_version_info* (*libxl_get_version_info)(libxl_ctx *ctx);
  void (*libxl_mac_copy)(libxl_ctx *ctx, libxl_mac *dst, libxl_mac *src);
  int (*libxl_mac_to_device_nic)(libxl_ctx *ctx, uint32_t domid,
                              const char *mac, libxl_device_nic *nic);
  void (*libxl_string_list_copy)(libxl_ctx *ctx, libxl_string_list *dst,
                              libxl_string_list *src);
  void (*libxl_uuid_generate)(libxl_uuid *uuid);
  void (*xtl_logger_destroy)(struct xentoollog_logger *logger);
  const char *(*xtl_level_to_string)(xentoollog_level);
  void (*xtl_log)(struct xentoollog_logger *logger,
             xentoollog_level level,
             int errnoval,
             const char *context,
             const char *format,
             ...) __attribute__((format(printf,5,6)));
} hyperxl_xen_fn;

static hyperxl_xen_fn xen_fns;

static inline int hyperxl_load_libraries() {
    void* handle = NULL;
    char* error = NULL;

    handle = dlopen("libxenlight.so", RTLD_LAZY);
    if (!handle)
        return -1;

    xen_fns.libxl_bitmap_count_set = dlsym(handle, "libxl_bitmap_count_set");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_bitmap_set = dlsym(handle, "libxl_bitmap_set");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_childproc_reaped = dlsym(handle, "libxl_childproc_reaped");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_childproc_setmode = dlsym(handle, "libxl_childproc_setmode");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_cpu_bitmap_alloc = dlsym(handle, "libxl_cpu_bitmap_alloc");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_ctx_alloc = dlsym(handle, "libxl_ctx_alloc");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_ctx_free = dlsym(handle, "libxl_ctx_free");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_defbool_set = dlsym(handle, "libxl_defbool_set");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_device_disk_add = dlsym(handle, "libxl_device_disk_add");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_device_disk_dispose = dlsym(handle, "libxl_device_disk_dispose");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_device_disk_init = dlsym(handle, "libxl_device_disk_init");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_device_disk_remove = dlsym(handle, "libxl_device_disk_remove");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_device_nic_add = dlsym(handle, "libxl_device_nic_add");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_device_nic_dispose = dlsym(handle, "libxl_device_nic_dispose");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_device_nic_init = dlsym(handle, "libxl_device_nic_init");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_device_nic_remove = dlsym(handle, "libxl_device_nic_remove");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_domain_build_info_init_type = dlsym(handle, "libxl_domain_build_info_init_type");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_domain_config_dispose = dlsym(handle, "libxl_domain_config_dispose");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_domain_config_init = dlsym(handle, "libxl_domain_config_init");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_domain_create_info_init = dlsym(handle, "libxl_domain_create_info_init");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_domain_create_new = dlsym(handle, "libxl_domain_create_new");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_domain_destroy = dlsym(handle, "libxl_domain_destroy");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_domain_info = dlsym(handle, "libxl_domain_info");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_domain_unpause = dlsym(handle, "libxl_domain_unpause");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_evdisable_domain_death = dlsym(handle, "libxl_evdisable_domain_death");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_evenable_domain_death = dlsym(handle, "libxl_evenable_domain_death");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_event_free = dlsym(handle, "libxl_event_free");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_event_register_callbacks = dlsym(handle, "libxl_event_register_callbacks");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_get_free_memory = dlsym(handle, "libxl_get_free_memory");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_get_required_shadow_memory = dlsym(handle, "libxl_get_required_shadow_memory");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_get_version_info = dlsym(handle, "libxl_get_version_info");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_mac_copy = dlsym(handle, "libxl_mac_copy");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_mac_to_device_nic = dlsym(handle, "libxl_mac_to_device_nic");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_string_list_copy = dlsym(handle, "libxl_string_list_copy");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.libxl_uuid_generate = dlsym(handle, "libxl_uuid_generate");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }

    handle = dlopen("libxenctrl.so", RTLD_LAZY);
    if (!handle){
      return -1;
    }
    xen_fns.xtl_logger_destroy = dlsym(handle, "xtl_logger_destroy");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.xtl_level_to_string = dlsym(handle, "xtl_level_to_string");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }
    xen_fns.xtl_log = dlsym(handle, "xtl_log");
    if ((error = dlerror()) != NULL)  {
      return -1;
    }

    return 0;
}

static inline int  hyperxl_initialize_driver(hyperxl_driver** pdriver, bool verbose) {

#ifndef LIBXL_HAVE_BUILDINFO_KERNEL

    return -1;

#else

    hyperxl_driver *driver;
    const libxl_version_info* version = NULL;
    uint32_t mem = 0;
    xentoollog_level log_level = XTL_INFO;

    *pdriver = (hyperxl_driver*)malloc(sizeof(hyperxl_driver));
    if ( *pdriver == NULL ) {
        return -1;
    }

    driver = *pdriver;

    if (verbose) {
        log_level = XTL_DEBUG;
    }
    driver->logger = (xentoollog_logger*)xtl_createlogger_hyperxl(log_level, 0);
    if (driver->logger == NULL) {
        goto release_driver;
    }

    if(xen_fns.libxl_ctx_alloc(&driver->ctx, LIBXL_VERSION, 0, driver->logger)) {
        goto close_logger;
    }

    xen_fns.libxl_childproc_setmode(driver->ctx, &libxl_child_hooks, driver->ctx);

    version = xen_fns.libxl_get_version_info(driver->ctx);
    if (version == NULL) {
        goto free_ctx;
    }

    driver->version = version->xen_version_major * 1000000 + version->xen_version_minor * 1000;
    driver->capabilities = strdup(version->capabilities);

    if(xen_fns.libxl_get_free_memory(driver->ctx, &mem)) {
        goto free_ctx;
    }

    xen_fns.libxl_event_register_callbacks(driver->ctx, &ev_hooks, driver->ctx);

    return 0;

free_ctx:
    xen_fns.libxl_ctx_free(driver->ctx);
close_logger:
    xen_fns.xtl_logger_destroy(driver->logger);
release_driver:
    free(driver);
    driver = NULL;
    return -1;

#endif //LIBXL_HAVE_BUILDINFO_KERNEL
}

static inline int hyperxl_ctx_free(libxl_ctx* ctx) {
    return xen_fns.libxl_ctx_free(ctx);
}

static inline int  hyperxl_domain_start(libxl_ctx* ctx, hyperxl_domain_config* config) {
	int i, ret = -1;
	uint32_t domid = 0;
	libxl_domain_config d_config;

	xen_fns.libxl_domain_config_init(&d_config);

	//init create info
	libxl_domain_create_info* c_info = &d_config.c_info;
	xen_fns.libxl_domain_create_info_init(c_info);

	if (config->hvm)
		c_info->type = LIBXL_DOMAIN_TYPE_HVM;
	else
		c_info->type = LIBXL_DOMAIN_TYPE_PV;

	xen_fns.libxl_uuid_generate(&c_info->uuid);
	c_info->name = strdup(config->name);
	xen_fns.libxl_defbool_set(&c_info->run_hotplug_scripts, false);

	//init_build_info
	libxl_domain_build_info* b_info = &d_config.b_info;
	if (config->hvm)
		xen_fns.libxl_domain_build_info_init_type(b_info, LIBXL_DOMAIN_TYPE_HVM);
	else {
		// currently only hvm is supported. pv mode will be enabled
		// whenever we can insert several serial ports and filesystem
		// into pv domain.
		goto cleanup;
	}

	// currently, we do not change vcpu and memory only, will add this
	// feature later.
	b_info->max_vcpus = config->max_vcpus;
    if (xen_fns.libxl_cpu_bitmap_alloc(ctx, &b_info->avail_vcpus, config->max_vcpus))
        goto cleanup;
    libxl_bitmap_set_none(&b_info->avail_vcpus);
    for (i = 0; i < config->max_vcpus; i++)
        xen_fns.libxl_bitmap_set((&b_info->avail_vcpus), i);

    b_info->sched_params.weight = 1000;
    b_info->max_memkb = config->max_memory_kb;
    b_info->target_memkb = config->max_memory_kb;
    b_info->video_memkb = 0;

    // currently, we only initialize hvm fields
    if (config->hvm) {
        xen_fns.libxl_defbool_set(&b_info->u.hvm.pae, true);
        xen_fns.libxl_defbool_set(&b_info->u.hvm.apic, false);
        xen_fns.libxl_defbool_set(&b_info->u.hvm.acpi, true);

        b_info->u.hvm.boot = strdup("c");

        b_info->cmdline = strdup(config->cmdline);
        b_info->kernel  = strdup(config->kernel);
        b_info->ramdisk = strdup(config->initrd);

        b_info->u.hvm.vga.kind = LIBXL_VGA_INTERFACE_TYPE_NONE;
        xen_fns.libxl_defbool_set(&b_info->u.hvm.nographic, 1);
        xen_fns.libxl_defbool_set(&b_info->u.hvm.vnc.enable, 0);
        xen_fns.libxl_defbool_set(&b_info->u.hvm.sdl.enable, 0);

        b_info->u.hvm.serial = strdup(config->console_sock);

        xen_fns.libxl_string_list_copy(ctx, &b_info->extra, (libxl_string_list*)config->extra);

        // comments from libvirt and libxenlight:
        //
        // The following comment and calculation were taken directly from
        // libxenlight's internal function xen_fns.libxl_get_required_shadow_memory():
        //
        // 256 pages (1MB) per vcpu, plus 1 page per MiB of RAM for the P2M map,
        // plus 1 page per MiB of RAM to shadow the resident processes.

        b_info->shadow_memkb = 4 * (256 * xen_fns.libxl_bitmap_count_set(&b_info->avail_vcpus) +
                                    2 * (b_info->max_memkb / 1024));
    }

    if (xen_fns.libxl_domain_create_new(ctx, &d_config,
                                      &domid, NULL, NULL)) {
    	goto cleanup;
    }

    libxl_evgen_domain_death* e_death = NULL;
    if (xen_fns.libxl_evenable_domain_death(ctx, domid, 0, &e_death)) {
    	goto cleanup;
    }

    xen_fns.libxl_domain_unpause(ctx, domid);
    config->domid = domid;
    config->ev    = e_death;

	ret = 0;

cleanup:
	xen_fns.libxl_domain_config_dispose(&d_config);
	return ret;
}

static inline void hyperxl_domain_cleanup(libxl_ctx *ctx, void* ev) {
    if (ev != NULL)
        xen_fns.libxl_evdisable_domain_death(ctx, (libxl_evgen_domain_death*)ev);
}

static inline void hyperxl_sigchld_handler(libxl_ctx* ctx) {
    int status, res;
    pid_t pid = waitpid(-1, &status, WNOHANG);
    printf("got child pid: %d\n", pid);
    if (pid > 0) {
        res = xen_fns.libxl_childproc_reaped(ctx, pid, status);
        printf("check whether child proc is created by libxl: %d\n", res);
    }
}

static inline void hyperxl_domain_event_handler(void *data, HYPERXL_EVENT_CONST libxl_event *event) {
    libxl_ctx *ctx = (libxl_ctx *)data;
    libxl_shutdown_reason xl_reason = event->u.domain_shutdown.shutdown_reason;

    if (event->type != LIBXL_EVENT_TYPE_DOMAIN_SHUTDOWN) {
        goto ignore;
    }

    if (xl_reason == LIBXL_SHUTDOWN_REASON_SUSPEND)
        goto ignore;

    DomainDeath_cgo((uint32_t)event->domid);

ignore:
    xen_fns.libxl_event_free(ctx, (libxl_event *)event);
}

static inline int  hyperxl_domain_destroy(libxl_ctx* ctx, uint32_t domid) {
    return xen_fns.libxl_domain_destroy(ctx, domid, NULL);
}

static inline int  hyperxl_domaim_check(libxl_ctx* ctx, uint32_t domid)
{
    return xen_fns.libxl_domain_info(ctx, NULL, domid);
}

// libxl internal in xen_fns.libxl__device_nic_add()
static inline int hyperxl_nic_add(libxl_ctx* ctx, uint32_t domid, hyperxl_nic_config* config) {

    libxl_device_nic nic;
    libxl_mac mac;
    int i, ret = -1;

    xen_fns.libxl_device_nic_init(&nic);
    nic.backend_domid = 0;
    nic.mtu = 1492;
    nic.model = strdup("e1000");
    nic.ip = strdup(config->ip);
    nic.bridge = strdup(config->bridge);
    nic.nictype = LIBXL_NIC_TYPE_VIF_IOEMU;
    nic.ifname = strdup(config->ifname);
    nic.gatewaydev = strdup(config->gatewaydev);
    if (config->mac != NULL) {
        for (i=0; i<6; i++) {
            mac[i] = (uint8_t)(*(config->mac + i));
        }
        xen_fns.libxl_mac_copy(ctx, &nic.mac, &mac);
    }

    if( xen_fns.libxl_device_nic_add(ctx, domid, &nic, 0) ) {
        goto cleanup;
    }

    ret = 0;

cleanup:
    xen_fns.libxl_device_nic_dispose(&nic);
    return ret;
}

static inline int hyperxl_nic_remove(libxl_ctx* ctx, uint32_t domid, const char* mac) {
    libxl_device_nic nic;
    int ret = -1;

    xen_fns.libxl_device_nic_init(&nic);
    if(xen_fns.libxl_mac_to_device_nic(ctx, domid, mac, &nic)) {
        char* msg = "failed to get device from mac";
        hyperxl_log_cgo(msg, strlen(msg));
        goto cleanup;
    }

    if(xen_fns.libxl_device_nic_remove(ctx, domid, &nic, 0)) {
        char* msg = "failed to remove nic from domain";
        hyperxl_log_cgo(msg, strlen(msg));
        goto cleanup;
    }
    ret = 0;

cleanup:
    xen_fns.libxl_device_nic_dispose(&nic);
    return ret;
}

static inline  void hyperxl_config_disk(hyperxl_disk_config* config, libxl_device_disk* disk) {
    xen_fns.libxl_device_disk_init(disk);
    disk->pdev_path = strdup(config->source);
    disk->vdev = strdup(config->target);
    disk->format = config->format;
    disk->backend = config->backend;
    disk->removable = 1;
    disk->readwrite = 1;
    disk->is_cdrom  = 0;
}

static inline int hyperxl_disk_add(libxl_ctx* ctx, uint32_t domid,hyperxl_disk_config* config) {
    libxl_device_disk disk;
    int ret = -1;
    hyperxl_config_disk(config, &disk);
    if (xen_fns.libxl_device_disk_add(ctx, domid, &disk, 0) ) {
        goto cleanup;
    }
    ret = 0;
cleanup:
    xen_fns.libxl_device_disk_dispose(&disk);
    return ret;
}

static inline int hyperxl_disk_remove(libxl_ctx* ctx, uint32_t domid,hyperxl_disk_config* config) {
    libxl_device_disk disk;
    int ret = -1;
    hyperxl_config_disk(config, &disk);
    if (xen_fns.libxl_device_disk_remove(ctx, domid, &disk, 0) ) {
        goto cleanup;
    }
    ret = 0;
cleanup:
    xen_fns.libxl_device_disk_dispose(&disk);
    return ret;
}

#define HYPERXL_LOG_BUF_SIZE 1024
static char hyperxl_log_buf[HYPERXL_LOG_BUF_SIZE];

struct xentoollog_logger_hyperxl {
    xentoollog_logger vtable;
    xentoollog_level min_level;
    size_t log_pos;
    unsigned flags;
    int progress_erase_len, progress_last_percent;
};

static inline  void hyperxl_log(xentoollog_logger_hyperxl *lg) {
    if (lg->log_pos > 0) {
        hyperxl_log_cgo(hyperxl_log_buf, (int)lg->log_pos);
        lg->log_pos = 0;
    }
}

static inline  void progress_erase(xentoollog_logger_hyperxl *lg) {
    if (lg->progress_erase_len && lg->log_pos < HYPERXL_LOG_BUF_SIZE ) {
        lg->log_pos += snprintf(hyperxl_log_buf + lg->log_pos, HYPERXL_LOG_BUF_SIZE - lg->log_pos, "\r%*s\r", lg->progress_erase_len, "");
    }
}

static inline  void hyperxl_vmessage(xentoollog_logger *logger_in,
                                 xentoollog_level level,
                                 int errnoval,
                                 const char *context,
                                 const char *format,
                                 va_list al) {
    xentoollog_logger_hyperxl *lg = (void*)logger_in;

    if (level < lg->min_level)
        return;

    progress_erase(lg);

    if (lg->log_pos < HYPERXL_LOG_BUF_SIZE)
        lg->log_pos += snprintf(hyperxl_log_buf + lg->log_pos, HYPERXL_LOG_BUF_SIZE - lg->log_pos, "%s: ", xen_fns.xtl_level_to_string(level));

    if (lg->log_pos < HYPERXL_LOG_BUF_SIZE)
        lg->log_pos += vsnprintf(hyperxl_log_buf + lg->log_pos, HYPERXL_LOG_BUF_SIZE - lg->log_pos, format, al);

    if (errnoval >= 0 && lg->log_pos < HYPERXL_LOG_BUF_SIZE)
        lg->log_pos += snprintf(hyperxl_log_buf + lg->log_pos, HYPERXL_LOG_BUF_SIZE - lg->log_pos, ": %s", strerror(errnoval));

    hyperxl_log(lg);
}

static inline  void hyperxl_message(struct xentoollog_logger *logger_in,
                                xentoollog_level level,
                                const char *context,
                                const char *format, ...)
{
    va_list al;
    va_start(al,format);
    hyperxl_vmessage(logger_in, level, -1, context, format, al);
    va_end(al);
}

static inline  void hyperxl_progress(struct xentoollog_logger *logger_in,
                                 const char *context,
                                 const char *doing_what, int percent,
                                 unsigned long done, unsigned long total) {
    xentoollog_logger_hyperxl *lg = (void*)logger_in;
    int newpel, extra_erase;
    xentoollog_level this_level;

    if (lg->flags & XTL_STDIOSTREAM_HIDE_PROGRESS)
        return;

    if (percent < lg->progress_last_percent) {
        this_level = XTL_PROGRESS;
    } else if (percent == lg->progress_last_percent) {
        return;
    } else if (percent < lg->progress_last_percent + 5) {
        this_level = XTL_DETAIL;
    } else {
        this_level = XTL_PROGRESS;
    }

    if (this_level < lg->min_level)
        return;

    lg->progress_last_percent = percent;

    hyperxl_message(logger_in, this_level, context,
                        "%s: %lu/%lu  %3d%%",
                        doing_what, done, total, percent);
}

static inline  void hyperxl_destroy(struct xentoollog_logger *logger_in) {
    xentoollog_logger_hyperxl *lg = (void*)logger_in;
    progress_erase(lg);
    free(lg);
}

#define HYPER_XTL_NEW_LOGGER(LOGGER,buffer) ({                                \
    xentoollog_logger_##LOGGER *new_consumer;                           \
                                                                        \
    (buffer).vtable.vmessage = LOGGER##_vmessage;                       \
    (buffer).vtable.progress = LOGGER##_progress;                       \
    (buffer).vtable.destroy  = LOGGER##_destroy;                        \
                                                                        \
    new_consumer = malloc(sizeof(*new_consumer));                       \
    if (!new_consumer) {                                                \
        xen_fns.xtl_log((xentoollog_logger*)&buffer,                            \
                XTL_CRITICAL, errno, "xtl",                             \
                "failed to allocate memory for new message logger");    \
    } else {                                                            \
        *new_consumer = buffer;                                         \
    }                                                                   \
                                                                        \
    new_consumer;                                                       \
});


static inline xentoollog_logger_hyperxl* xtl_createlogger_hyperxl
        (xentoollog_level min_level, unsigned flags) {
    xentoollog_logger_hyperxl newlogger;

    newlogger.min_level = min_level;
    newlogger.flags = flags;

    newlogger.log_pos = 0;

    newlogger.progress_erase_len = 0;
    newlogger.progress_last_percent = 0;

    return HYPER_XTL_NEW_LOGGER(hyperxl, newlogger);
}
*/
import "C"

import (
	"errors"
	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor"
	"unsafe"
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
		ev:            unsafe.Pointer(nil),
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

func loadXenLib() error {
	res := (int)(C.hyperxl_load_libraries())
	if res == 0 {
		return nil
	}
	return errors.New("Fail to load xen libraries")
}

//int  hyperxl_initialize_driver(hyperxl_driver** pdriver);
func HyperxlInitializeDriver() (*XenDriver, int) {

	var driver *C.struct_hyperxl_driver = nil
	res := (int)(C.hyperxl_initialize_driver(&driver, (C.bool)(glog.V(1))))
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
func HyperxlDomainStart(ctx LibxlCtxPtr, config *DomainConfig) (int, unsafe.Pointer, int) {
	cc := config.toC()
	res := (int)(C.hyperxl_domain_start((*C.struct_libxl__ctx)(ctx), cc))
	return (int)(cc.domid), cc.ev, res
}

func HyperDomainCleanup(ctx LibxlCtxPtr, ev unsafe.Pointer) {
	C.hyperxl_domain_cleanup((*C.struct_libxl__ctx)(ctx), ev)
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
		mac:        (*C.char)(unsafe.Pointer(&mac[0])),
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

//int hyperxl_ctx_free(libxl_ctx *ctx /* 0 is OK */);
func LibxlCtxFree(ctx LibxlCtxPtr) int {
	return (int)(C.hyperxl_ctx_free((*C.struct_libxl__ctx)(ctx)))
}

/******************************************************************************
 *  Callbacks
 ******************************************************************************/

//export DomainDeath_cgo
func DomainDeath_cgo(domid C.uint32_t) {
	defer func() { recover() }() //in case the vmContext or channel has been released
	dom := (uint32)(domid)
	glog.Infof("got xen hypervisor message: domain %d quit", dom)
	if vm, ok := globalDriver.domains[dom]; ok {
		glog.V(1).Infof("Domain %d managed by xen driver, try close it")
		delete(globalDriver.domains, dom)
		vm.Hub <- &hypervisor.VmExit{}
		HyperDomainCleanup(globalDriver.Ctx, vm.DCtx.(*XenContext).ev)
	}
}

//export hyperxl_log_cgo
func hyperxl_log_cgo(msg *C.char, len C.int) {
	if glog.V(1) {
		glog.Info("[libxl] ", C.GoStringN(msg, len))
	}
}
