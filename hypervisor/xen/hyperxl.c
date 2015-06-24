#include "hyperxl.h"

#ifdef WITH_XEN

static const libxl_childproc_hooks libxl_child_hooks = {
    .chldowner = libxl_sigchld_owner_mainloop,
};

const struct libxl_event_hooks ev_hooks = {
    .event_occurs_mask = LIBXL_EVENTMASK_ALL,
    .event_occurs = hyperxl_domain_event_handler,
    .disaster = NULL, //disaster means the libxl has some issue, rather than a domain
};

typedef struct xentoollog_logger_hyperxl xentoollog_logger_hyperxl;
xentoollog_logger_hyperxl* xtl_createlogger_hyperxl
        (xentoollog_level min_level, unsigned flags);

int  hyperxl_initialize_driver(hyperxl_driver** pdriver) {

#ifndef LIBXL_HAVE_BUILDINFO_KERNEL

    return -1;

#else

    hyperxl_driver *driver;
    const libxl_version_info* version = NULL;
    uint32_t mem = 0;

    *pdriver = (hyperxl_driver*)malloc(sizeof(hyperxl_driver));
    if ( *pdriver == NULL ) {
        return -1;
    }

    driver = *pdriver;

    driver->logger = (xentoollog_logger*)xtl_createlogger_hyperxl(XTL_DEBUG, 0);
    if (driver->logger == NULL) {
        goto release_driver;
    }

    if(libxl_ctx_alloc(&driver->ctx, LIBXL_VERSION, 0, driver->logger)) {
        goto close_logger;
    }

    libxl_childproc_setmode(driver->ctx, &libxl_child_hooks, driver->ctx);

    version = libxl_get_version_info(driver->ctx);
    if (version == NULL) {
        goto free_ctx;
    }

    driver->version = version->xen_version_major * 1000000 + version->xen_version_minor * 1000;
    driver->capabilities = strdup(version->capabilities);

    if(libxl_get_free_memory(driver->ctx, &mem)) {
        goto free_ctx;
    }

    libxl_event_register_callbacks(driver->ctx, &ev_hooks, driver);

    return 0;

free_ctx:
    libxl_ctx_free(driver->ctx);
close_logger:
    xtl_logger_destroy(driver->logger);
release_driver:
    free(driver);
    driver = NULL;
    return -1;

#endif //LIBXL_HAVE_BUILDINFO_KERNEL
}

int  hyperxl_domain_start(libxl_ctx* ctx, hyperxl_domain_config* config) {
	int i, ret = -1;
	uint32_t domid = 0;
	libxl_domain_config d_config;

	libxl_domain_config_init(&d_config);

	//init create info
	libxl_domain_create_info* c_info = &d_config.c_info;
	libxl_domain_create_info_init(c_info);

	if (config->hvm)
		c_info->type = LIBXL_DOMAIN_TYPE_HVM;
	else
		c_info->type = LIBXL_DOMAIN_TYPE_PV;

	libxl_uuid_generate(&c_info->uuid);
	c_info->name = strdup(config->name);
	libxl_defbool_set(&c_info->run_hotplug_scripts, false);

	//init_build_info
	libxl_domain_build_info* b_info = &d_config.b_info;
	if (config->hvm)
		libxl_domain_build_info_init_type(b_info, LIBXL_DOMAIN_TYPE_HVM);
	else {
		// currently only hvm is supported. pv mode will be enabled
		// whenever we can insert several serial ports and filesystem
		// into pv domain. 
		goto cleanup;
	}

	// currently, we do not change vcpu and memory only, will add this
	// feature later.
	b_info->max_vcpus = config->max_vcpus;
    if (libxl_cpu_bitmap_alloc(ctx, &b_info->avail_vcpus, config->max_vcpus))
        goto cleanup;
    libxl_bitmap_set_none(&b_info->avail_vcpus);
    for (i = 0; i < config->max_vcpus; i++)
        libxl_bitmap_set((&b_info->avail_vcpus), i);

    b_info->sched_params.weight = 1000;
    b_info->max_memkb = config->max_memory_kb;
    b_info->target_memkb = config->max_memory_kb;
    b_info->video_memkb = 0;

    // currently, we only initialize hvm fields
    if (config->hvm) {
        libxl_defbool_set(&b_info->u.hvm.pae, true);
        libxl_defbool_set(&b_info->u.hvm.apic, false);
        libxl_defbool_set(&b_info->u.hvm.acpi, true);

        b_info->u.hvm.boot = strdup("c");

        b_info->cmdline = strdup(config->cmdline);
        b_info->kernel  = strdup(config->kernel);
        b_info->ramdisk = strdup(config->initrd);

        b_info->u.hvm.vga.kind = LIBXL_VGA_INTERFACE_TYPE_NONE;
        libxl_defbool_set(&b_info->u.hvm.nographic, 1);
        libxl_defbool_set(&b_info->u.hvm.vnc.enable, 0);
        libxl_defbool_set(&b_info->u.hvm.sdl.enable, 0);

        b_info->u.hvm.serial = strdup(config->console_sock);

        libxl_string_list_copy(ctx, &b_info->extra, (libxl_string_list*)config->extra);

        /*
         * comments from libvirt and libxenlight:
         *
         * The following comment and calculation were taken directly from
         * libxenlight's internal function libxl_get_required_shadow_memory():
         *
         * 256 pages (1MB) per vcpu, plus 1 page per MiB of RAM for the P2M map,
         * plus 1 page per MiB of RAM to shadow the resident processes.
         */
        b_info->shadow_memkb = 4 * (256 * libxl_bitmap_count_set(&b_info->avail_vcpus) +
                                    2 * (b_info->max_memkb / 1024));
    }

    if (libxl_domain_create_new(ctx, &d_config,
                                      &domid, NULL, NULL)) {
    	goto cleanup;
    }

    libxl_evgen_domain_death* e_death = NULL;
    if (libxl_evenable_domain_death(ctx, domid, 0, &e_death)) {
    	goto cleanup;
    }

    libxl_domain_unpause(ctx, domid);
    config->domid = domid;

	ret = 0;

cleanup:
	libxl_domain_config_dispose(&d_config);
	return ret;
}

void hyperxl_sigchld_handler(libxl_ctx* ctx) {
    int status, res;
    pid_t pid = waitpid(-1, &status, WNOHANG);
    printf("got child pid: %d\n", pid);
    if (pid > 0) {
        res = libxl_childproc_reaped(ctx, pid, status);
        printf("check whether child proc is created by libxl: %d\n", res);
    }
}

void hyperxl_domain_event_handler(void *data, HYPERXL_EVENT_CONST libxl_event *event) {
    hyperxl_driver* driver = (hyperxl_driver*)data;
    libxl_shutdown_reason xl_reason = event->u.domain_shutdown.shutdown_reason;

    if (event->type != LIBXL_EVENT_TYPE_DOMAIN_SHUTDOWN) {
        goto ignore;
    }

    if (xl_reason == LIBXL_SHUTDOWN_REASON_SUSPEND)
        goto ignore;

    DomainDeath_cgo((uint32_t)event->domid);

ignore:
    libxl_event_free(driver->ctx, (libxl_event *)event);
}

int  hyperxl_domain_destroy(libxl_ctx* ctx, uint32_t domid) {
    return libxl_domain_destroy(ctx, domid, NULL);
}

int  hyperxl_domaim_check(libxl_ctx* ctx, uint32_t domid)
{
    return libxl_domain_info(ctx, NULL, domid);
}

// libxl internal in libxl__device_nic_add()
int hyperxl_nic_add(libxl_ctx* ctx, uint32_t domid, hyperxl_nic_config* config) {

    libxl_device_nic nic;
    int ret = -1;

    libxl_device_nic_init(&nic);
    nic.backend_domid = 0;
    nic.mtu = 1492;
    nic.model = strdup("e1000");
    nic.ip = strdup(config->ip);
    nic.bridge = strdup(config->bridge);
    nic.nictype = LIBXL_NIC_TYPE_VIF_IOEMU;
    nic.ifname = strdup(config->ifname);
    nic.gatewaydev = strdup(config->gatewaydev);
    libxl_mac_copy(ctx, &nic.mac, (libxl_mac*)&config->mac);

    if( libxl_device_nic_add(ctx, domid, &nic, 0) ) {
        goto cleanup;
    }

    ret = 0;

cleanup:
    libxl_device_nic_dispose(&nic);
    return ret;
}

int hyperxl_nic_remove(libxl_ctx* ctx, uint32_t domid, const char* mac) {
    libxl_device_nic nic;
    int ret = -1;

    libxl_device_nic_init(&nic);
    if(libxl_mac_to_device_nic(ctx, domid, mac, &nic)) {
        goto cleanup;
    }

    if(libxl_device_nic_remove(ctx, domid, &nic, 0)) {
        goto cleanup;
    }
    ret = 0;

cleanup:
    libxl_device_nic_dispose(&nic);
    return ret;
}

static void hyperxl_config_disk(hyperxl_disk_config* config, libxl_device_disk* disk) {
    libxl_device_disk_init(disk);
    disk->pdev_path = strdup(config->source);
    disk->vdev = strdup(config->target);
    disk->format = config->format;
    disk->backend = config->backend;
    disk->removable = 1;
    disk->readwrite = 1;
    disk->is_cdrom  = 0;
}

int hyperxl_disk_add(libxl_ctx* ctx, uint32_t domid,hyperxl_disk_config* config) {
    libxl_device_disk disk;
    int ret = -1;
    hyperxl_config_disk(config, &disk);
    if (libxl_device_disk_add(ctx, domid, &disk, 0) ) {
        goto cleanup;
    }
    ret = 0;
cleanup:
    libxl_device_disk_dispose(&disk);
    return ret;
}

int hyperxl_disk_remove(libxl_ctx* ctx, uint32_t domid,hyperxl_disk_config* config) {
    libxl_device_disk disk;
    int ret = -1;
    hyperxl_config_disk(config, &disk);
    if (libxl_device_disk_remove(ctx, domid, &disk, 0) ) {
        goto cleanup;
    }
    ret = 0;
cleanup:
    libxl_device_disk_dispose(&disk);
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

static void hyperxl_log(xentoollog_logger_hyperxl *lg) {
    if (lg->log_pos > 0) {
        hyperxl_log_cgo(hyperxl_log_buf, (int)lg->log_pos);
        lg->log_pos = 0;
    }
}

static void progress_erase(xentoollog_logger_hyperxl *lg) {
    if (lg->progress_erase_len && lg->log_pos < HYPERXL_LOG_BUF_SIZE ) {
        lg->log_pos += snprintf(hyperxl_log_buf + lg->log_pos, HYPERXL_LOG_BUF_SIZE - lg->log_pos, "\r%*s\r", lg->progress_erase_len, "");
    }
}

static void hyperxl_vmessage(xentoollog_logger *logger_in,
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
        lg->log_pos += snprintf(hyperxl_log_buf + lg->log_pos, HYPERXL_LOG_BUF_SIZE - lg->log_pos, "%s: ", xtl_level_to_string(level));

    if (lg->log_pos < HYPERXL_LOG_BUF_SIZE)
        lg->log_pos += vsnprintf(hyperxl_log_buf + lg->log_pos, HYPERXL_LOG_BUF_SIZE - lg->log_pos, format, al);

    if (errnoval >= 0 && lg->log_pos < HYPERXL_LOG_BUF_SIZE)
        lg->log_pos += snprintf(hyperxl_log_buf + lg->log_pos, HYPERXL_LOG_BUF_SIZE - lg->log_pos, ": %s", strerror(errnoval));

    hyperxl_log(lg);
}

static void hyperxl_message(struct xentoollog_logger *logger_in,
                                xentoollog_level level,
                                const char *context,
                                const char *format, ...)
{
    va_list al;
    va_start(al,format);
    hyperxl_vmessage(logger_in, level, -1, context, format, al);
    va_end(al);
}

static void hyperxl_progress(struct xentoollog_logger *logger_in,
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

static void hyperxl_destroy(struct xentoollog_logger *logger_in) {
    xentoollog_logger_hyperxl *lg = (void*)logger_in;
    progress_erase(lg);
    free(lg);
}

xentoollog_logger_hyperxl* xtl_createlogger_hyperxl
        (xentoollog_level min_level, unsigned flags) {
    xentoollog_logger_hyperxl newlogger;

    newlogger.min_level = min_level;
    newlogger.flags = flags;

    newlogger.log_pos = 0;

    newlogger.progress_erase_len = 0;
    newlogger.progress_last_percent = 0;

    return XTL_NEW_LOGGER(hyperxl, newlogger);
}

#else //WITH_XEN

int  hyperxl_initialize_driver(hyperxl_driver** pdriver) {
    return -1;
}

void hyperxl_destroy_driver(hyperxl_driver* driver);

int  hyperxl_domain_start(libxl_ctx* ctx, hyperxl_domain_config* config){
    return -1;
}

int  hyperxl_domain_destroy(libxl_ctx* ctx, uint32_t domid){
   return -1;
}

int  hyperxl_domaim_check(libxl_ctx* ctx, uint32_t domid)
{
    return -1;
}

void hyperxl_sigchld_handler(libxl_ctx* ctx){}

void hyperxl_domain_event_handler(void *data, HYPERXL_EVENT_CONST libxl_event *event){}

int hyperxl_nic_add(libxl_ctx* ctx, uint32_t domid, hyperxl_nic_config* config){
   return -1;
}
int hyperxl_nic_remove(libxl_ctx* ctx, uint32_t domid, const char* mac){
   return -1;
}

int hyperxl_disk_add(libxl_ctx* ctx, uint32_t domid,hyperxl_disk_config* config){
    return -1;
}
int hyperxl_disk_remove(libxl_ctx* ctx, uint32_t domid,hyperxl_disk_config* config){
   return -1;
}

int libxl_ctx_free(libxl_ctx *ctx /* 0 is OK */){
    return -1;
}

int libxl_event_free(libxl_ctx *ctx, libxl_event *event){
   return -1;
}

#endif //WITH_XEN
