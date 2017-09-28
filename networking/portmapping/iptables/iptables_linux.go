package iptables

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/hyperhq/hypercontainer-utils/hlog"
)

type Action string
type Table string

const (
	Append Action = "-A"
	Delete Action = "-D"
	Insert Action = "-I"
	Nat    Table  = "nat"
	Filter Table  = "filter"
	Mangle Table  = "mangle"
)

var (
	iptablesPath        string
	supportsXlock       = false
	ErrIptablesNotFound = errors.New("Iptables not found")
)

type Chain struct {
	Name   string
	Bridge string
	Table  Table
}

type ChainError struct {
	Chain  string
	Output []byte
}

func (e *ChainError) Error() string {
	return fmt.Sprintf("Error iptables %s: %s", e.Chain, string(e.Output))
}

func initCheck() error {
	if iptablesPath == "" {
		path, err := exec.LookPath("iptables")
		if err != nil {
			return ErrIptablesNotFound
		}
		iptablesPath = path
		supportsXlock = exec.Command(iptablesPath, "--wait", "-L", "-n").Run() == nil
	}
	return nil
}

// Check if a dnat rule exists
func OperatePortMap(action Action, chain string, rule []string) error {
	if output, err := Raw(append([]string{
		"-t", string(Nat), string(action), chain}, rule...)...); err != nil {
		return fmt.Errorf("Unable to setup network port map: %s", err)
	} else if len(output) != 0 {
		return &ChainError{Chain: chain, Output: output}
	}

	return nil
}

func PortMapExists(chain string, rule []string) bool {
	// iptables -C, --check option was added in v.1.4.11
	// http://ftp.netfilter.org/pub/iptables/changes-iptables-1.4.11.txt

	// try -C
	// if exit status is 0 then return true, the rule exists
	if _, err := Raw(append([]string{
		"-t", "nat", "-C", chain}, rule...)...); err == nil {
		return true
	}

	return false
}

var hostPortRulePattern = regexp.MustCompile(`.* -p ([cdtpu]{3}) .* --dport ([0-9]{1,5})(:([0-9]{1,5}))?`)

func PortMapUsed(chain string, proto string, begin, end int) bool {
	// parse "iptables -S" for the rule (this checks rules in a specific chain
	// in a specific table)
	outputs, _ := exec.Command("iptables", "-t", "nat", "-S", chain).Output()
	existingRules := bytes.NewBuffer(outputs)
	var fin = false
	for !fin {
		rule, err := existingRules.ReadString(byte('\n'))
		if err == io.EOF {
			fin = true
		} else if err != nil {
			break
		}

		match := hostPortRulePattern.FindStringSubmatch(rule)
		if len(match) < 5 {
			hlog.Log(hlog.TRACE, "pass short rule line: %v", rule)
			continue
		}

		p := match[1]
		if p != proto {
			continue
		}

		if match[2] == "" {
			continue
		}

		pt, err := strconv.ParseInt(match[2], 10, 32)
		if err != nil {
			continue
		}
		p1 := int(pt)

		p2 := p1
		if match[4] != "" {
			pt, err = strconv.ParseInt(match[4], 10, 32)
			if err != nil {
				continue
			}
			p2 = int(pt)
		}

		if (begin >= p1 && begin <= p2) || (end >= p1 && end <= p2) || (begin < p1 && end > p2) {
			return true
		}
	}

	return false
}

// Check if a rule exists
func Exists(table Table, chain string, rule ...string) bool {
	if string(table) == "" {
		table = Filter
	}

	// iptables -C, --check option was added in v.1.4.11
	// http://ftp.netfilter.org/pub/iptables/changes-iptables-1.4.11.txt

	// try -C
	// if exit status is 0 then return true, the rule exists
	if _, err := Raw(append([]string{
		"-t", string(table), "-C", chain}, rule...)...); err == nil {
		return true
	}

	// parse "iptables -S" for the rule (this checks rules in a specific chain
	// in a specific table)
	ruleString := strings.Join(rule, " ")
	existingRules, _ := exec.Command("iptables", "-t", string(table), "-S", chain).Output()

	// regex to replace ips in rule
	// because MASQUERADE rule will not be exactly what was passed
	re := regexp.MustCompile(`[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\/[0-9]{1,2}`)

	return strings.Contains(
		re.ReplaceAllString(string(existingRules), "?"),
		re.ReplaceAllString(ruleString, "?"),
	)
}

// Call 'iptables' system command, passing supplied arguments
func Raw(args ...string) ([]byte, error) {
	if err := initCheck(); err != nil {
		return nil, err
	}
	if supportsXlock {
		args = append([]string{"--wait"}, args...)
	}

	hlog.Log(hlog.TRACE, "%s, %v", iptablesPath, args)

	output, err := exec.Command(iptablesPath, args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("iptables failed: iptables %v: %s (%s)", strings.Join(args, " "), output, err)
	}

	// ignore iptables' message about xtables lock
	if strings.Contains(string(output), "waiting for it to exit") {
		output = []byte("")
	}

	return output, err
}
