package types

import (
	"fmt"
	"os"
	"strings"

	"github.com/Unknwon/goconfig"
	"github.com/hyperhq/hypercontainer-utils/hlog"
	"github.com/hyperhq/hyperd/utils"
)

type HyperConfig struct {
	ConfigFile string

	Root            string
	Host            string
	GRPCHost        string
	StorageDriver   string
	VmFactoryPolicy string
	Driver          string
	Kernel          string
	Initrd          string
	Bridge          string
	BridgeIP        string
	DisableIptables bool
	DefaultLog      string
	DefaultLogOpt   map[string]string

	logPrefix string
}

func NewHyperConfig(config string) *HyperConfig {
	if config == "" {
		config = "/etc/hyper/config"
	}
	hlog.Log(hlog.INFO, "config file: ", config)

	c := &HyperConfig{
		ConfigFile: config,
		Root:       "/var/lib/hyper",
		logPrefix:  fmt.Sprintf("[%s] ", config),
	}

	cfg, err := goconfig.LoadConfigFile(config)
	if err != nil {
		c.Log(hlog.ERROR, "read config file failed: %v", err)
		return nil
	}

	hyperRoot, _ := cfg.GetValue(goconfig.DEFAULT_SECTION, "Root")
	if hyperRoot != "" {
		c.Root = hyperRoot
	}

	c.StorageDriver, _ = cfg.GetValue(goconfig.DEFAULT_SECTION, "StorageDriver")
	c.Kernel, _ = cfg.GetValue(goconfig.DEFAULT_SECTION, "Kernel")
	c.Initrd, _ = cfg.GetValue(goconfig.DEFAULT_SECTION, "Initrd")
	c.Bridge, _ = cfg.GetValue(goconfig.DEFAULT_SECTION, "Bridge")
	c.BridgeIP, _ = cfg.GetValue(goconfig.DEFAULT_SECTION, "BridgeIP")
	c.Host, _ = cfg.GetValue(goconfig.DEFAULT_SECTION, "Host")
	driver, _ := cfg.GetValue(goconfig.DEFAULT_SECTION, "Hypervisor")
	c.Driver = strings.ToLower(driver)
	c.DisableIptables = cfg.MustBool(goconfig.DEFAULT_SECTION, "DisableIptables", false)
	c.DefaultLog, _ = cfg.GetValue(goconfig.DEFAULT_SECTION, "Logger")
	c.DefaultLogOpt, _ = cfg.GetSection("Log")
	c.VmFactoryPolicy, _ = cfg.GetValue(goconfig.DEFAULT_SECTION, "VmFactoryPolicy")
	c.GRPCHost, _ = cfg.GetValue(goconfig.DEFAULT_SECTION, "gRPCHost")

	c.Log(hlog.INFO, "config items: %#v", c)
	return c
}

func (c *HyperConfig) AdvertiseEnv() {
	utils.HYPER_ROOT = c.Root

	os.Setenv("HYPER_CONFIG", c.ConfigFile)
}

func (c *HyperConfig) LogPrefix() string {
	return c.logPrefix
}

func (c *HyperConfig) Log(level hlog.LogLevel, args ...interface{}) {
	hlog.HLog(level, c, 1, args...)
}
