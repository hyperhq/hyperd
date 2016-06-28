package client

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/hyperhq/hyperd/utils"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/lib/term"

	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/docker/registry"
	"github.com/docker/engine-api/types"
	registrytypes "github.com/docker/engine-api/types/registry"
	"gopkg.in/yaml.v2"
	"net/http"
)

type AuthRequest func(authConfig types.AuthConfig) (io.ReadCloser, string, int, error)

func (cli *HyperClient) requestWithLogin(index *registrytypes.IndexInfo, op AuthRequest, opTag string) (io.ReadCloser, string, int, error) {

	authConfig := registry.ResolveAuthConfig(cli.configFile.AuthConfigs, index)
	body, ctype, statusCode, err := op(authConfig)
	if statusCode == http.StatusUnauthorized {
		fmt.Fprintf(cli.out, "\nPlease login prior to %s:\n", opTag)
		if err = cli.HyperCmdLogin(registry.GetAuthConfigKey(index)); err != nil {
			return nil, "", -1, err
		}
		authConfig = registry.ResolveAuthConfig(cli.configFile.AuthConfigs, index)
		return op(authConfig)
	}
	return body, ctype, statusCode, err
}

func (cli *HyperClient) readStreamOutput(body io.ReadCloser, contentType string, setRawTerminal bool, stdout, stderr io.Writer) error {
	defer body.Close()

	if utils.MatchesContentType(contentType, "application/json") {
		return jsonmessage.DisplayJSONMessagesStream(body, stdout, cli.outFd, cli.isTerminalOut, nil)
	}
	if stdout != nil || stderr != nil {
		// When TTY is ON, use regular copy
		var err error
		if setRawTerminal {
			_, err = io.Copy(stdout, body)
		} else {
			_, err = stdcopy.StdCopy(stdout, stderr, body)
		}
		return err
	}
	return nil
}

func (cli *HyperClient) resizeTty(containerId, execId string) {
	height, width := cli.getTtySize()
	if height == 0 && width == 0 {
		return
	}
	cli.client.WinResize(containerId, execId, height, width)
}

func (cli *HyperClient) monitorTtySize(containerId, execId string) error {
	//cli.resizeTty(id, tag)

	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGWINCH)
	go func() {
		for range sigchan {
			cli.resizeTty(containerId, execId)
		}
	}()
	return nil
}

func (cli *HyperClient) getTtySize() (int, int) {
	if !cli.isTerminalOut {
		return 0, 0
	}
	ws, err := term.GetWinsize(cli.outFd)
	if err != nil {
		fmt.Printf("Error getting size: %s", err.Error())
		if ws == nil {
			return 0, 0
		}
	}
	return int(ws.Height), int(ws.Width)
}

func (cli *HyperClient) GetTag() string {
	return utils.RandStr(8, "alphanum")
}

func (cli *HyperClient) ConvertYamlToJson(yamlBody []byte) ([]byte, error) {
	var userPod pod.UserPod
	if err := yaml.Unmarshal(yamlBody, &userPod); err != nil {
		return []byte(""), err
	}
	jsonBody, err := json.Marshal(&userPod)
	if err != nil {
		return []byte(""), err
	}
	return jsonBody, nil
}
