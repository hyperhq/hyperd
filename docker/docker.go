// Build this with the docker configuration
package docker

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"hyper/lib/glog"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"time"
)

// Now, the Hyper will not support the TLS with docker.
// It is under development.  So the Hyper and docker should be deployed
// in same machine.
const (
	defaultTrustKeyFile = "key.json"
	defaultCaFile       = "ca.pem"
	defaultKeyFile      = "key.pem"
	defaultCertFile     = "cert.pem"
	defaultHostAddress  = "unix:///var/run/docker.sock"
	defaultProto        = "unix"
	dockerClientVersion = "1.17"
)

// Define some common configuration of the Docker daemon
type DockerConfig struct {
	host         string
	address      string
	trustKeyFile string
	caFile       string
	keyFile      string
	certFile     string
	debugMode    int
	tlsConfig    *tls.Config
}

type DockerCli struct {
	proto        string
	scheme       string
	dockerConfig *DockerConfig
	transport    *http.Transport
}

func NewDockerCli(keyFile string, proto, addr string, tlsConfig *tls.Config) *DockerCli {
	var (
		scheme       = "http"
		dockerConfig DockerConfig
	)

	if tlsConfig != nil {
		scheme = "https"
	}

	tr := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	timeout := 32 * time.Second
	if proto == "unix" {
		tr.DisableCompression = true
		tr.Dial = func(_, _ string) (net.Conn, error) {
			return net.DialTimeout(proto, addr, timeout)
		}
	} else {
		tr.Proxy = http.ProxyFromEnvironment
		tr.Dial = (&net.Dialer{Timeout: timeout}).Dial
	}

	dockerConfig.host = ""
	dockerConfig.address = addr
	if keyFile == "" {
		dockerConfig.keyFile = defaultKeyFile
	} else {
		dockerConfig.keyFile = keyFile
	}
	dockerConfig.certFile = defaultCertFile
	dockerConfig.caFile = defaultCaFile
	dockerConfig.trustKeyFile = defaultTrustKeyFile
	dockerConfig.debugMode = 1
	dockerConfig.tlsConfig = tlsConfig

	return &DockerCli{
		proto:        proto,
		scheme:       scheme,
		dockerConfig: &dockerConfig,
		transport:    tr,
	}
}

func (cli *DockerCli) ExecDockerCmd(args ...string) ([]byte, int, error) {
	command := args[0]
	switch command {
	case "info":
		return cli.SendCmdInfo(args[1])
	case "create":
		return cli.SendCmdCreate(args[1])
	default:
		return nil, -1, errors.New("This cmd is not supported!\n")
	}
	return nil, -1, errors.New("The ExecDockerCmd function is done!\n")
}

func (cli *DockerCli) HTTPClient() *http.Client {
	return &http.Client{Transport: cli.transport}
}

func (cli *DockerCli) encodeData(data interface{}) (*bytes.Buffer, error) {
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

func (cli *DockerCli) clientRequest(method, path string, in io.Reader, headers map[string][]string) (io.ReadCloser, string, int, error) {
	if in == nil {
		in = bytes.NewReader([]byte{})
	}
	req, err := http.NewRequest(method, fmt.Sprintf("/v%s%s", dockerClientVersion, path), in)
	if err != nil {
		return nil, "", -1, err
	}
	req.Header.Set("User-Agent", "Docker-client/"+dockerClientVersion)
	req.URL.Host = cli.dockerConfig.address
	req.URL.Scheme = cli.scheme
	if headers != nil {
		for k, v := range headers {
			req.Header[k] = v
		}
	}

	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "text/plain")
	}

	resp, err := cli.HTTPClient().Do(req)
	statusCode := -1
	if resp != nil {
		statusCode = resp.StatusCode
	}

	if err != nil {
		if strings.Contains(err.Error(), "connection refused") {
			return nil, "", statusCode, err
		}

		if cli.dockerConfig.tlsConfig == nil {
			return nil, "", statusCode, fmt.Errorf("Are you trying to connect with a TLS-enabled daemon without TLS, %v", err)
		}
		return nil, "", statusCode, fmt.Errorf("An error encountered while connecting: %v", err)
	}

	if statusCode < 200 || statusCode >= 400 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, "", statusCode, err
		}

		return nil, "", statusCode, fmt.Errorf("An error encountered returned from Docker daemon, %s\n", bytes.TrimSpace(body))
	}
	glog.V(3).Info("Finish the client request\n")
	return resp.Body, resp.Header.Get("Content-Type"), statusCode, nil
}

func (cli *DockerCli) Call(method, path string, data interface{}, headers map[string][]string) (io.ReadCloser, int, error) {
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

func readBody(stream io.ReadCloser, statusCode int, err error) ([]byte, int, error) {
	if stream != nil {
		defer stream.Close()
	}

	if err != nil {
		return nil, statusCode, err
	}
	body, err := ioutil.ReadAll(stream)
	if err != nil {
		return nil, -1, err
	}
	return body, statusCode, nil
}

func (cli *DockerCli) Stream(method, path string, in io.Reader, stdout io.Writer, headers map[string][]string) error {
	glog.V(3).Info("Ready to get the response from docker daemon\n")
	body, contentType, _, err := cli.clientRequest(method, path, in, headers)
	if err != nil {
		return err
	}
	defer body.Close()
	glog.V(3).Info("Process the response data\n")
	if strings.Contains(contentType, "application/json") {
		glog.V(3).Info("Process the response data with JSON\n")
		return DisplayJSONMessagesStream(body, stdout, 0, true)
	}

	glog.V(3).Info("Process the response data with pure copy\n")
	if stdout != nil {
		_, err := io.Copy(stdout, body)
		if err != nil {
			return err
		}
	}
	return nil
}
