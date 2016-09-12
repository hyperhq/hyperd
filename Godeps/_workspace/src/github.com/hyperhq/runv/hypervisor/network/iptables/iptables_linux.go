package iptables

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/golang/glog"
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

func PortMapUsed(chain string, rule []string) bool {
	// parse "iptables -S" for the rule (this checks rules in a specific chain
	// in a specific table)
	existingRules, _ := exec.Command("iptables", "-t", "nat", "-S", chain).Output()
	ruleString := strings.Join(rule, " ")

	glog.V(3).Infof("MapUsed %s", ruleString)
	// regex to replace ips in rule
	re := regexp.MustCompile(`[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\:[0-9]{1,2}`)

	return strings.Contains(
		re.ReplaceAllString(string(existingRules), "?"),
		re.ReplaceAllString(ruleString, "?"),
	)
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

	glog.V(3).Infof("%s, %v", iptablesPath, args)

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
