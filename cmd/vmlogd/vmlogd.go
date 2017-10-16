package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/hyperhq/hyperd/utils"
	"github.com/hyperhq/runv/hypervisor"
)

const (
	VmLogDir = "/var/log/hyper/vm"
)

func main() {
	logdir := flag.String("logdir", VmLogDir, "hyper vm log directroy")
	flHelp := flag.Bool("help", false, "Print help message for vmlog daemon")
	flVersion := flag.Bool("version", false, "Version Message")
	flag.Usage = func() { printHelp() }
	flag.Parse()
	if *flHelp == true {
		printHelp()
		return
	}

	if *flVersion == true {
		printVersion()
		return
	}

	if os.Geteuid() != 0 {
		fmt.Printf("The VmLogd daemon needs to be run as root\n")
		return
	}
	os.MkdirAll(*logdir, 0755)

	ln, err := net.Listen("unix", hypervisor.VmLogdSock)
	if err != nil {
		fmt.Printf("fail to listen to %v: %v\n", hypervisor.VmLogdSock, err)
		return
	}
	defer os.Remove(hypervisor.VmLogdSock)

	var msg hypervisor.LogMessage

	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Printf("fail to accept connection: %v\n", err)
			break
		}

		if err := json.NewDecoder(conn).Decode(&msg); err != nil {
			fmt.Printf("fail to receive message: %v\n", err)
			conn.Close()
			continue
		}

		fmt.Printf("got message: %v\n", msg)
		if msg.Message != "start" {
			conn.Close()
			continue
		}

		msg.Message = "success"
		if err := json.NewEncoder(conn).Encode(&msg); err != nil {
			fmt.Printf("fail to send message: %v\n", err)
			conn.Close()
			continue
		}

		conn.Close()
		go handleVmOutput(*logdir, msg.Id, msg.Path)
	}
}

func UnixSocketConnect(addr string) (conn net.Conn, err error) {
	for i := 0; i < 500; i++ {
		time.Sleep(20 * time.Millisecond)
		conn, err = net.Dial("unix", addr)
		if err == nil {
			return
		}
	}

	return
}

func handleVmOutput(logdir, id, path string) {
	vmlogfile, err := os.OpenFile(filepath.Join(logdir, id), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0640)
	if err != nil {
		fmt.Printf("fail to create vm log file for %v: %v\n", id, err)
		return
	}

	defer func() {
		vmlogfile.WriteString(fmt.Sprintf("\nend of vmlog: %v\n", time.Now()))
		vmlogfile.Close()
		os.Rename(filepath.Join(logdir, id), filepath.Join(logdir, id+"-destroyed"))
	}()

	conn, err := UnixSocketConnect(path)
	if err != nil {
		fmt.Printf("fail to connect to %v: %v\n", path, err)
		return
	}
	fmt.Printf("connected to %v\n", path)
	buf := make([]byte, 512)
	for {
		nr, err := conn.Read(buf)
		if err != nil || nr < 0 {
			fmt.Printf("fail to read %v: %v\n", path, err)
			break
		}

		vmlogfile.Write(buf[:nr])
	}
}

func printHelp() {
	var helpMessage = `Usage:
  %s [OPTIONS]

Application Options:
  --logdir              Log directory

Help Options:
  -h, --help             Show this help message

`
	fmt.Printf(helpMessage, os.Args[0])
}

func printVersion() {
	fmt.Printf("The version is %s\n", utils.VERSION)
}
