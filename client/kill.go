package client

import (
	"fmt"
	"strconv"
	"strings"
	"syscall"

	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdKill(args ...string) error {
	var opts struct {
		Pod    bool   `short:"p" long:"pod" default:"false" default-mask:"-" description:"kill all containers in a pod"`
		Signal string `short:"s" long:"signal" value-name:"\"\"" description:"The signal to kill containers, default is 9"`
	}
	var parser = gflag.NewParser(&opts, gflag.Default|gflag.IgnoreUnknown)
	parser.Usage = "kill [OPTIONS] CONTAINER_ID|POD_ID\n\nSend kill signal to container or Pod"
	args, err := parser.ParseArgs(args)
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}

	if len(args) == 0 {
		return fmt.Errorf("\"kill\" requires a minimum of 1 argument, please provide container ID.\n")
	}

	sig := 9
	if opts.Signal != "" {
		s, err := parseSignal(opts.Signal)
		if err != nil {
			return err
		}
		sig = int(s)
	}

	for i := range args {
		if opts.Pod {
			err = cli.client.KillPod(args[i], sig)
			if err != nil {
				fmt.Fprintf(cli.err, "failed to kill pod %s: %v", args[i], err)
			}
		} else {
			err = cli.client.KillContainer(args[i], sig)
			if err != nil {
				fmt.Fprintf(cli.err, "failed to kill container %s: %v", args[i], err)
			}
		}
	}

	return nil
}

func parseSignal(rawSignal string) (syscall.Signal, error) {
	s, err := strconv.Atoi(rawSignal)
	if err == nil {
		return syscall.Signal(s), nil
	}
	signal, ok := linuxSignalMap[strings.TrimPrefix(strings.ToUpper(rawSignal), "SIG")]
	if !ok {
		return -1, fmt.Errorf("unknown signal %q", rawSignal)
	}
	return signal, nil
}

var linuxSignalMap = map[string]syscall.Signal{
	"ABRT":   syscall.SIGABRT,
	"ALRM":   syscall.SIGALRM,
	"BUS":    syscall.SIGBUS,
	"CHLD":   syscall.SIGCHLD,
	"CLD":    syscall.SIGCLD,
	"CONT":   syscall.SIGCONT,
	"FPE":    syscall.SIGFPE,
	"HUP":    syscall.SIGHUP,
	"ILL":    syscall.SIGILL,
	"INT":    syscall.SIGINT,
	"IO":     syscall.SIGIO,
	"IOT":    syscall.SIGIOT,
	"KILL":   syscall.SIGKILL,
	"PIPE":   syscall.SIGPIPE,
	"POLL":   syscall.SIGPOLL,
	"PROF":   syscall.SIGPROF,
	"PWR":    syscall.SIGPWR,
	"QUIT":   syscall.SIGQUIT,
	"SEGV":   syscall.SIGSEGV,
	"STKFLT": syscall.SIGSTKFLT,
	"STOP":   syscall.SIGSTOP,
	"SYS":    syscall.SIGSYS,
	"TERM":   syscall.SIGTERM,
	"TRAP":   syscall.SIGTRAP,
	"TSTP":   syscall.SIGTSTP,
	"TTIN":   syscall.SIGTTIN,
	"TTOU":   syscall.SIGTTOU,
	"UNUSED": syscall.SIGUNUSED,
	"URG":    syscall.SIGURG,
	"USR1":   syscall.SIGUSR1,
	"USR2":   syscall.SIGUSR2,
	"VTALRM": syscall.SIGVTALRM,
	"WINCH":  syscall.SIGWINCH,
	"XCPU":   syscall.SIGXCPU,
	"XFSZ":   syscall.SIGXFSZ,
}
