package client

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/hyperhq/hyper/lib/docker/cliconfig"
	"github.com/hyperhq/hyper/lib/docker/pkg/jsonmessage"
	"github.com/hyperhq/hyper/lib/docker/pkg/stdcopy"
	"github.com/hyperhq/hyper/lib/docker/registry"
	"github.com/hyperhq/hyper/utils"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/lib/term"

	"gopkg.in/yaml.v2"
)

var (
	ErrConnectionRefused = errors.New("Cannot connect to the Hyper daemon. Is 'hyperd' running on this host?")
)

func (cli *HyperClient) HTTPClient() *http.Client {
	return &http.Client{Transport: cli.transport}
}

func (cli *HyperClient) encodeData(data interface{}) (*bytes.Buffer, error) {
	params := bytes.NewBuffer(nil)
	if data != nil {
		buf, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}
		if _, err := params.Write(buf); err != nil {
			return nil, err
		}
	}
	return params, nil
}

func (cli *HyperClient) clientRequest(method, path string, in io.Reader, headers map[string][]string) (io.ReadCloser, string, int, error) {
	expectedPayload := (method == "POST" || method == "PUT" || method == "DELETE")
	if expectedPayload && in == nil {
		in = bytes.NewReader([]byte{})
	}
	req, err := http.NewRequest(method, fmt.Sprintf("/v%s%s", utils.VERSION, path), in)
	if err != nil {
		return nil, "", -1, err
	}
	req.Header.Set("User-Agent", "Hyper-Client/"+utils.VERSION)
	req.URL.Host = cli.addr
	req.URL.Scheme = cli.scheme

	if headers != nil {
		for k, v := range headers {
			req.Header[k] = v
		}
	}

	if expectedPayload && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "text/plain")
	}

	resp, err := cli.HTTPClient().Do(req)
	statusCode := -1
	if resp != nil {
		statusCode = resp.StatusCode
	}
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") {
			return nil, "", statusCode, ErrConnectionRefused
		}

		return nil, "", statusCode, fmt.Errorf("An error occurred trying to connect: %v", err)
	}

	if statusCode < 200 || statusCode >= 400 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, "", statusCode, err
		}
		if len(body) == 0 {
			return nil, "", statusCode, fmt.Errorf("Error: request returned %s for API route and version %s, check if the server supports the requested API version", http.StatusText(statusCode), req.URL)
		}
		return nil, "", statusCode, fmt.Errorf("Error from daemon's response: %s", bytes.TrimSpace(body))
	}

	return resp.Body, resp.Header.Get("Content-Type"), statusCode, nil
}

func (cli *HyperClient) clientRequestAttemptLogin(method, path string, in io.Reader, out io.Writer, index *registry.IndexInfo, cmdName string) (io.ReadCloser, int, error) {
	cmdAttempt := func(authConfig cliconfig.AuthConfig) (io.ReadCloser, int, error) {
		buf, err := json.Marshal(authConfig)
		if err != nil {
			return nil, -1, err
		}
		registryAuthHeader := []string{
			base64.URLEncoding.EncodeToString(buf),
		}

		// begin the request
		body, contentType, statusCode, err := cli.clientRequest(method, path, in, map[string][]string{
			"X-Registry-Auth": registryAuthHeader,
		})
		if err == nil && out != nil {
			// If we are streaming output, complete the stream since
			// errors may not appear until later.
			err = cli.streamBody(body, contentType, true, out, nil)
		}
		if err != nil {
			// Since errors in a stream appear after status 200 has been written,
			// we may need to change the status code.
			if strings.Contains(err.Error(), "Authentication is required") ||
				strings.Contains(err.Error(), "Status 401") ||
				strings.Contains(err.Error(), "status code 401") {
				statusCode = http.StatusUnauthorized
			}
		}
		return body, statusCode, err
	}

	// Resolve the Auth config relevant for this server
	authConfig := registry.ResolveAuthConfig(cli.configFile, index)
	body, statusCode, err := cmdAttempt(authConfig)
	if statusCode == http.StatusUnauthorized {
		fmt.Fprintf(cli.out, "\nPlease login prior to %s:\n", cmdName)
		if err = cli.HyperCmdLogin(index.GetAuthConfigKey()); err != nil {
			return nil, -1, err
		}
		authConfig = registry.ResolveAuthConfig(cli.configFile, index)
		return cmdAttempt(authConfig)
	}
	return body, statusCode, err
}
func (cli *HyperClient) call(method, path string, data interface{}, headers map[string][]string) (io.ReadCloser, int, error) {
	params, err := cli.encodeData(data)
	if err != nil {
		return nil, -1, err
	}

	if data != nil {
		if headers == nil {
			headers = make(map[string][]string)
		}
		headers["Content-Type"] = []string{"application/json"}
	}

	body, _, statusCode, err := cli.clientRequest(method, path, params, headers)
	return body, statusCode, err
}

func (cli *HyperClient) stream(method, path string, in io.Reader, out io.Writer, headers map[string][]string) error {
	return cli.streamHelper(method, path, true, in, out, nil, headers)
}

func (cli *HyperClient) streamHelper(method, path string, setRawTerminal bool, in io.Reader, stdout, stderr io.Writer, headers map[string][]string) error {
	body, contentType, _, err := cli.clientRequest(method, path, in, headers)
	if err != nil {
		return err
	}
	return cli.streamBody(body, contentType, setRawTerminal, stdout, stderr)
}

func (cli *HyperClient) streamBody(body io.ReadCloser, contentType string, setRawTerminal bool, stdout, stderr io.Writer) error {
	defer body.Close()

	if utils.MatchesContentType(contentType, "application/json") {
		return jsonmessage.DisplayJSONMessagesStream(body, stdout, cli.outFd, cli.isTerminalOut)
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

func readBody(stream io.ReadCloser, statusCode int, err error) ([]byte, int, error) {
	if stream != nil {
		defer stream.Close()
	}
	if err != nil {
		return nil, statusCode, err
	}
	if stream == nil {
		return nil, statusCode, err
	}
	body, err := ioutil.ReadAll(stream)
	if err != nil {
		return nil, -1, err
	}
	return body, statusCode, nil
}

func (cli *HyperClient) resizeTty(id, tag string) {
	height, width := cli.getTtySize()
	if height == 0 && width == 0 {
		return
	}
	v := url.Values{}
	v.Set("h", strconv.Itoa(height))
	v.Set("w", strconv.Itoa(width))
	v.Set("id", id)
	v.Set("tag", tag)

	if _, _, err := readBody(cli.call("POST", "/tty/resize?"+v.Encode(), nil, nil)); err != nil {
		//fmt.Printf("Error resize: %s", err.Error())
	}
}
func (cli *HyperClient) monitorTtySize(id, tag string) error {
	//cli.resizeTty(id, tag)

	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGWINCH)
	go func() {
		for range sigchan {
			cli.resizeTty(id, tag)
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
