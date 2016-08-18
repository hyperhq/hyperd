package daemon

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"syscall"

	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/utils"
)

// Record Structure for a single host record
type Record struct {
	Hosts string
	IP    string
}

// WriteTo writes record to file and returns bytes written or error
func (r Record) WriteTo(w io.Writer) (int64, error) {
	n, err := fmt.Fprintf(w, "%s\t%s\n", r.IP, r.Hosts)
	return int64(n), err
}

var (
	// Default hosts config records slice
	defaultHosts = []Record{
		{Hosts: "localhost", IP: "127.0.0.1"},
		{Hosts: "localhost ip6-localhost ip6-loopback", IP: "::1"},
		{Hosts: "ip6-localnet", IP: "fe00::0"},
		{Hosts: "ip6-mcastprefix", IP: "ff00::0"},
		{Hosts: "ip6-allnodes", IP: "ff02::1"},
		{Hosts: "ip6-allrouters", IP: "ff02::2"},
	}

	defaultHostsFilename = "hosts"
)

func generateDefaultHosts() ([]byte, error) {
	content := bytes.NewBuffer(nil)

	for _, r := range defaultHosts {
		if _, err := r.WriteTo(content); err != nil {
			return nil, err
		}
	}

	return content.Bytes(), nil
}

// prepareHosts creates hosts file for given pod
func prepareHosts(podID string) (string, error) {
	var hostsDir = path.Join(utils.HYPER_ROOT, "hosts", podID)
	var hostsPath = path.Join(hostsDir, defaultHostsFilename)
	var err error

	if err = os.MkdirAll(hostsDir, 0755); err != nil && !os.IsExist(err) {
		return "", err
	}

	if _, err = os.Stat(hostsPath); err != nil && os.IsNotExist(err) {
		// mount tmpfs on hostsDir
		if err := syscall.Mount(podID+"-hosts", hostsDir, "tmpfs", 0, "size=1024K"); err != nil {
			return "", err
		}

		hostsContent, err := generateDefaultHosts()
		if err != nil {
			return "", err
		}
		return hostsPath, ioutil.WriteFile(hostsPath, hostsContent, 0644)
	}

	return hostsPath, nil
}

func cleanupHosts(podID string) error {
	var hostsDir = path.Join(utils.HYPER_ROOT, "hosts", podID)
	var hostsPath = path.Join(hostsDir, defaultHostsFilename)

	glog.Infof("cleanupHosts %s, %s", hostsDir, hostsPath)
	_, err := os.Stat(hostsPath)
	if err != nil {
		if os.IsNotExist(err) {
			glog.Infof("cannot find %s", hostsPath)
			return nil
		}

		glog.Infof("cannot stat %s, err %v", hostsPath, err)
		return err
	}
	// try unmount hostsDir
	if err := syscall.Unmount(hostsDir, syscall.MNT_DETACH); err != nil {
		glog.Infof("unmount %s failed, %v", hostsDir, err)
		return err
	}

	return nil
}
