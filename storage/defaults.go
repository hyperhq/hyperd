package storage

const (
	DEFAULT_DM_POOL      string = "hyper-volume-pool"
	DEFAULT_DM_POOL_SIZE int    = 20971520 * 512
	DEFAULT_DM_DATA_LOOP string = "/dev/loop6"
	DEFAULT_DM_META_LOOP string = "/dev/loop7"
	DEFAULT_DM_VOL_SIZE  int    = 2 * 1024 * 1024 * 1024
	DEFAULT_VOL_FS              = "ext4"
	DEFAULT_VOL_MKFS            = "mkfs.ext4"
)
