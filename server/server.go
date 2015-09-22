package server

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"expvar"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/hyperhq/hyper/engine"
	dockertypes "github.com/hyperhq/hyper/lib/docker/api/types"
	"github.com/hyperhq/hyper/lib/docker/cliconfig"
	"github.com/hyperhq/hyper/lib/docker/pkg/ioutils"
	"github.com/hyperhq/hyper/lib/docker/pkg/streamformatter"
	"github.com/hyperhq/hyper/lib/portallocator"
	"github.com/hyperhq/hyper/lib/version"
	"github.com/hyperhq/hyper/types"
	"github.com/hyperhq/hyper/utils"
	"github.com/hyperhq/runv/lib/glog"

	"github.com/gorilla/mux"
)

var (
	activationLock chan struct{}
)

type HttpServer struct {
	srv *http.Server
	l   net.Listener
}

func (s *HttpServer) Serve() error {
	return s.srv.Serve(s.l)
}
func (s *HttpServer) Close() error {
	return s.l.Close()
}

type HttpApiFunc func(eng *engine.Engine, version version.Version, w http.ResponseWriter, r *http.Request, vars map[string]string) error

func hijackServer(w http.ResponseWriter) (io.ReadCloser, io.Writer, error) {
	conn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return nil, nil, err
	}
	// Flush the options to make sure the client sets the raw mode
	conn.Write([]byte{})
	return conn, conn, nil
}

func closeStreams(streams ...interface{}) {
	for _, stream := range streams {
		if tcpc, ok := stream.(interface {
			CloseWrite() error
		}); ok {
			tcpc.CloseWrite()
		} else if closer, ok := stream.(io.Closer); ok {
			closer.Close()
		}
	}
}

// Check to make sure request's Content-Type is application/json
func checkForJson(r *http.Request) error {
	ct := r.Header.Get("Content-Type")

	// No Content-Type header is ok as long as there's no Body
	if ct == "" {
		if r.Body == nil || r.ContentLength == 0 {
			return nil
		}
	}

	// Otherwise it better be json
	if utils.MatchesContentType(ct, "application/json") {
		return nil
	}
	return fmt.Errorf("Content-Type specified (%s) must be 'application/json'", ct)
}

//If we don't do this, POST method without Content-type (even with empty body) will fail
func parseForm(r *http.Request) error {
	if r == nil {
		return nil
	}
	if err := r.ParseForm(); err != nil && !strings.HasPrefix(err.Error(), "mime:") {
		return err
	}
	return nil
}

func parseMultipartForm(r *http.Request) error {
	if err := r.ParseMultipartForm(4096); err != nil && !strings.HasPrefix(err.Error(), "mime:") {
		return err
	}
	return nil
}

func httpError(w http.ResponseWriter, err error) {
	statusCode := http.StatusInternalServerError
	// FIXME: this is brittle and should not be necessary.
	// If we need to differentiate between different possible error types, we should
	// create appropriate error types with clearly defined meaning.
	errStr := strings.ToLower(err.Error())
	if strings.Contains(errStr, "no such") {
		statusCode = http.StatusNotFound
	} else if strings.Contains(errStr, "bad parameter") {
		statusCode = http.StatusBadRequest
	} else if strings.Contains(errStr, "conflict") {
		statusCode = http.StatusConflict
	} else if strings.Contains(errStr, "impossible") {
		statusCode = http.StatusNotAcceptable
	} else if strings.Contains(errStr, "wrong login/password") {
		statusCode = http.StatusUnauthorized
	} else if strings.Contains(errStr, "hasn't been activated") {
		statusCode = http.StatusForbidden
	}

	if err != nil {
		glog.Errorf("HTTP Error: statusCode=%d %v", statusCode, err)
		http.Error(w, err.Error(), statusCode)
	}
}

// writeJSONEnv writes the engine.Env values to the http response stream as a
// json encoded body.
func writeJSONEnv(w http.ResponseWriter, code int, v engine.Env) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	return v.Encode(w)
}

// writeJSON writes the value v to the http response stream as json with standard
// json encoding.
func writeJSON(w http.ResponseWriter, code int, v interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	return json.NewEncoder(w).Encode(v)
}

func streamJSON(job *engine.Job, w http.ResponseWriter, flush bool) {
	w.Header().Set("Content-Type", "application/json")
	job.Stdout.Add(w)
}

func getBoolParam(value string) (bool, error) {
	if value == "" {
		return false, nil
	}
	ret, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("Bad parameter")
	}
	return ret, nil
}

func getVersion(eng *engine.Engine, version version.Version, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	w.Header().Set("Content-Type", "application/json")
	eng.ServeHTTP(w, r)
	return nil
}

func getInfo(eng *engine.Engine, version version.Version, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	w.Header().Set("Content-Type", "application/json")
	eng.ServeHTTP(w, r)
	return nil
}

func getList(eng *engine.Engine, version version.Version, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := r.ParseForm(); err != nil {
		return nil
	}

	glog.V(1).Infof("List type is %s", r.Form.Get("item"))
	job := eng.Job("list", r.Form.Get("item"))
	stdoutBuf := bytes.NewBuffer(nil)

	job.Stdout.Add(stdoutBuf)

	if err := job.Run(); err != nil {
		return err
	}

	str := engine.Tail(stdoutBuf, 1)
	type listResponse struct {
		Item    string   `json:"item"`
		PodData []string `json:"podData"`
		VmData  []string `json:"vmData"`
		CData   []string `json:"cData"`
	}
	var res listResponse
	if err := json.Unmarshal([]byte(str), &res); err != nil {
		return err
	}
	var env engine.Env
	env.Set("Item", res.Item)
	if res.Item == "pod" {
		env.SetList("podData", res.PodData)
	}
	if res.Item == "vm" {
		env.SetList("vmData", res.VmData)
	}
	if res.Item == "container" {
		env.SetList("cData", res.CData)
	}
	return writeJSONEnv(w, http.StatusOK, env)
}

func getImages(eng *engine.Engine, version version.Version, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := r.ParseForm(); err != nil {
		return nil
	}

	job := eng.Job("images", r.Form.Get("all"))
	stdoutBuf := bytes.NewBuffer(nil)

	job.Stdout.Add(stdoutBuf)

	if err := job.Run(); err != nil {
		return err
	}

	str := engine.Tail(stdoutBuf, 1)
	type response struct {
		Images []string `json:"imagesList"`
	}
	var res response
	if err := json.Unmarshal([]byte(str), &res); err != nil {
		return err
	}
	var env engine.Env
	env.SetList("imagesList", res.Images)
	return writeJSONEnv(w, http.StatusOK, env)
}

func getPodInfo(eng *engine.Engine, version version.Version, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := r.ParseForm(); err != nil {
		return nil
	}

	job := eng.Job("podInfo", r.Form.Get("podName"))
	stdoutBuf := bytes.NewBuffer(nil)

	job.Stdout.Add(stdoutBuf)
	if err := job.Run(); err != nil {
		return err
	}

	var (
		dat             map[string]interface{}
		returnedJSONstr string
	)
	returnedJSONstr = engine.Tail(stdoutBuf, 1)
	if err := json.Unmarshal([]byte(returnedJSONstr), &dat); err != nil {
		return err
	}

	return writeJSON(w, http.StatusCreated, dat["data"])
}

func getContainerInfo(eng *engine.Engine, version version.Version, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := r.ParseForm(); err != nil {
		return nil
	}

	job := eng.Job("containerInfo", r.Form.Get("container"))
	stdoutBuf := bytes.NewBuffer(nil)

	job.Stdout.Add(stdoutBuf)
	if err := job.Run(); err != nil {
		return err
	}

	var (
		dat             map[string]interface{}
		returnedJSONstr string
	)
	returnedJSONstr = engine.Tail(stdoutBuf, 1)
	if err := json.Unmarshal([]byte(returnedJSONstr), &dat); err != nil {
		return err
	}

	return writeJSON(w, http.StatusCreated, dat["data"])
}

func postStop(eng *engine.Engine, version version.Version, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := r.ParseForm(); err != nil {
		return nil
	}

	glog.V(1).Infof("Stop the POD name is %s", r.Form.Get("podName"))
	job := eng.Job("podStop", r.Form.Get("podId"), r.Form.Get("stopVm"))
	stdoutBuf := bytes.NewBuffer(nil)
	job.Stdout.Add(stdoutBuf)

	if err := job.Run(); err != nil {
		return err
	}
	var (
		env             engine.Env
		dat             map[string]interface{}
		returnedJSONstr string
	)
	returnedJSONstr = engine.Tail(stdoutBuf, 1)
	if err := json.Unmarshal([]byte(returnedJSONstr), &dat); err != nil {
		return err
	}

	env.Set("ID", dat["ID"].(string))
	env.SetInt("Code", (int)(dat["Code"].(float64)))
	env.Set("Cause", dat["Cause"].(string))

	return writeJSONEnv(w, http.StatusOK, env)
}

func postExec(eng *engine.Engine, version version.Version, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := r.ParseForm(); err != nil {
		return nil
	}

	var (
		job                 = eng.Job("exec", r.Form.Get("type"), r.Form.Get("value"), r.Form.Get("command"), r.Form.Get("tag"))
		errOut    io.Writer = os.Stderr
		errStream io.Writer
	)

	// Setting up the streaming http interface.
	inStream, outStream, err := hijackServer(w)
	if err != nil {
		return err
	}
	defer closeStreams(inStream, outStream)

	fmt.Fprintf(outStream, "HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")

	errStream = outStream
	job.Stdin.Add(inStream)
	job.Stdout.Add(outStream)
	job.Stderr.Set(errStream)

	// Now run the user process in container.
	job.SetCloseIO(false)
	if err := job.Run(); err != nil {
		fmt.Fprintf(errOut, "Error starting exec command in POD %s: %s", r.Form.Get("podname"), err.Error())
		return err
	}
	w.WriteHeader(http.StatusNoContent)

	return nil
}

func postAttach(eng *engine.Engine, version version.Version, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := r.ParseForm(); err != nil {
		return nil
	}

	var (
		job                 = eng.Job("attach", r.Form.Get("type"), r.Form.Get("value"), r.Form.Get("tag"), r.Form.Get("remove"))
		errOut    io.Writer = os.Stderr
		errStream io.Writer
	)

	// Setting up the streaming http interface.
	inStream, outStream, err := hijackServer(w)
	if err != nil {
		return err
	}
	defer closeStreams(inStream, outStream)

	fmt.Fprintf(outStream, "HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")

	errStream = outStream
	job.Stdin.Add(inStream)
	job.Stdout.Add(outStream)
	job.Stderr.Set(errStream)

	// Now run the user process in container.
	job.SetCloseIO(false)
	if err := job.Run(); err != nil {
		fmt.Fprintf(errOut, "Error starting attach to POD %s: %s\n", r.Form.Get("podname"), err.Error())
		return err
	}
	w.WriteHeader(http.StatusNoContent)

	return nil
}

func postAuth(eng *engine.Engine, version version.Version, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	job := eng.Job("auth")
	job.Stdin.Add(r.Body)
	stdoutBuf := bytes.NewBuffer(nil)
	job.Stdout.Add(stdoutBuf)
	if err := job.Run(); err != nil {
		return err
	}
	var (
		dat             map[string]interface{}
		returnedJSONstr string
	)
	returnedJSONstr = engine.Tail(stdoutBuf, 1)
	if err := json.Unmarshal([]byte(returnedJSONstr), &dat); err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, &dockertypes.AuthResponse{
		Status: dat["status"].(string),
	})
}

func postContainerCreate(eng *engine.Engine, version version.Version, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := r.ParseForm(); err != nil {
		return nil
	}

	glog.V(1).Infof("Image name is %s", r.Form.Get("imageName"))
	job := eng.Job("create", r.Form.Get("imageName"))
	stdoutBuf := bytes.NewBuffer(nil)
	stderrBuf := bytes.NewBuffer(nil)

	job.Stdout.Add(stdoutBuf)
	job.Stderr.Add(stderrBuf)
	if err := job.Run(); err != nil {
		return err
	}

	var (
		env             engine.Env
		dat             map[string]interface{}
		returnedJSONstr string
	)
	returnedJSONstr = engine.Tail(stdoutBuf, 1)
	if err := json.Unmarshal([]byte(returnedJSONstr), &dat); err != nil {
		return err
	}

	env.Set("ContainerID", dat["ContainerID"].(string))
	return writeJSONEnv(w, http.StatusCreated, env)
}

func postContainerCommit(eng *engine.Engine, version version.Version, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := r.ParseForm(); err != nil {
		return nil
	}

	glog.V(1).Infof("container ID is %s", r.Form.Get("container"))
	job := eng.Job("commit", r.Form.Get("container"), r.Form.Get("repo"), r.Form.Get("author"), r.Form.Get("change"), r.Form.Get("message"), r.Form.Get("pause"))
	stdoutBuf := bytes.NewBuffer(nil)

	job.Stdout.Add(stdoutBuf)
	if err := job.Run(); err != nil {
		return err
	}
	var (
		env             engine.Env
		dat             map[string]interface{}
		returnedJSONstr string
	)
	returnedJSONstr = engine.Tail(stdoutBuf, 1)
	if err := json.Unmarshal([]byte(returnedJSONstr), &dat); err != nil {
		return err
	}

	env.Set("ID", dat["ID"].(string))
	env.SetInt("Code", (int)(dat["Code"].(float64)))
	env.Set("Cause", dat["Cause"].(string))

	return writeJSONEnv(w, http.StatusOK, env)
}

func postContainerRename(eng *engine.Engine, version version.Version, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := r.ParseForm(); err != nil {
		return nil
	}

	job := eng.Job("rename", r.Form.Get("oldName"), r.Form.Get("newName"))
	stdoutBuf := bytes.NewBuffer(nil)

	job.Stdout.Add(stdoutBuf)
	if err := job.Run(); err != nil {
		return err
	}
	var (
		env             engine.Env
		dat             map[string]interface{}
		returnedJSONstr string
	)
	returnedJSONstr = engine.Tail(stdoutBuf, 1)
	if err := json.Unmarshal([]byte(returnedJSONstr), &dat); err != nil {
		return err
	}

	env.Set("ID", dat["ID"].(string))
	env.SetInt("Code", (int)(dat["Code"].(float64)))
	env.Set("Cause", dat["Cause"].(string))

	return writeJSONEnv(w, http.StatusOK, env)
}

func postPodCreate(eng *engine.Engine, version version.Version, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := r.ParseForm(); err != nil {
		return nil
	}

	glog.V(1).Infof("Args string is %s", r.Form.Get("podArgs"))
	job := eng.Job("podCreate", r.Form.Get("podArgs"))
	stdoutBuf := bytes.NewBuffer(nil)

	job.Stdout.Add(stdoutBuf)

	if err := job.Run(); err != nil {
		return err
	}

	var (
		env             engine.Env
		dat             map[string]interface{}
		returnedJSONstr string
	)
	returnedJSONstr = engine.Tail(stdoutBuf, 1)
	if err := json.Unmarshal([]byte(returnedJSONstr), &dat); err != nil {
		return err
	}

	env.Set("ID", dat["ID"].(string))
	env.SetInt("Code", (int)(dat["Code"].(float64)))
	env.Set("Cause", dat["Cause"].(string))

	return writeJSONEnv(w, http.StatusOK, env)
}

func postPodStart(eng *engine.Engine, version version.Version, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := r.ParseForm(); err != nil {
		return nil
	}

	job := eng.Job("podStart", r.Form.Get("podId"), r.Form.Get("vmId"))
	stdoutBuf := bytes.NewBuffer(nil)
	job.Stdout.Add(stdoutBuf)

	if err := job.Run(); err != nil {
		return err
	}

	var (
		env             engine.Env
		dat             map[string]interface{}
		returnedJSONstr string
	)
	returnedJSONstr = engine.Tail(stdoutBuf, 1)
	if err := json.Unmarshal([]byte(returnedJSONstr), &dat); err != nil {
		return err
	}

	env.Set("ID", dat["ID"].(string))
	env.SetInt("Code", (int)(dat["Code"].(float64)))
	env.Set("Cause", dat["Cause"].(string))

	return writeJSONEnv(w, http.StatusOK, env)
}

func postPodRun(eng *engine.Engine, version version.Version, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := r.ParseForm(); err != nil {
		return nil
	}

	job := eng.Job("podRun", r.Form.Get("podArgs"), r.Form.Get("remove"))
	stdoutBuf := bytes.NewBuffer(nil)
	job.Stdout.Add(stdoutBuf)

	if err := job.Run(); err != nil {
		return err
	}

	var (
		env             engine.Env
		dat             map[string]interface{}
		returnedJSONstr string
	)
	returnedJSONstr = engine.Tail(stdoutBuf, 1)
	if err := json.Unmarshal([]byte(returnedJSONstr), &dat); err != nil {
		return err
	}

	env.Set("ID", dat["ID"].(string))
	env.SetInt("Code", (int)(dat["Code"].(float64)))
	env.Set("Cause", dat["Cause"].(string))

	return writeJSONEnv(w, http.StatusOK, env)
}

func postVmCreate(eng *engine.Engine, version version.Version, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := r.ParseForm(); err != nil {
		return nil
	}

	job := eng.Job("vmCreate", r.Form.Get("cpu"), r.Form.Get("mem"))
	stdoutBuf := bytes.NewBuffer(nil)

	job.Stdout.Add(stdoutBuf)

	if err := job.Run(); err != nil {
		return err
	}

	var (
		env             engine.Env
		dat             map[string]interface{}
		returnedJSONstr string
	)
	returnedJSONstr = engine.Tail(stdoutBuf, 1)
	if err := json.Unmarshal([]byte(returnedJSONstr), &dat); err != nil {
		return err
	}

	env.Set("ID", dat["ID"].(string))
	env.SetInt("Code", (int)(dat["Code"].(float64)))
	env.Set("Cause", dat["Cause"].(string))

	return writeJSONEnv(w, http.StatusOK, env)
}

func postVmKill(eng *engine.Engine, version version.Version, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := r.ParseForm(); err != nil {
		return nil
	}

	job := eng.Job("vmKill", r.Form.Get("vm"))
	stdoutBuf := bytes.NewBuffer(nil)

	job.Stdout.Add(stdoutBuf)

	if err := job.Run(); err != nil {
		return err
	}

	var (
		env             engine.Env
		dat             map[string]interface{}
		returnedJSONstr string
	)
	returnedJSONstr = engine.Tail(stdoutBuf, 1)
	if err := json.Unmarshal([]byte(returnedJSONstr), &dat); err != nil {
		return err
	}

	env.Set("ID", dat["ID"].(string))
	env.SetInt("Code", (int)(dat["Code"].(float64)))
	env.Set("Cause", dat["Cause"].(string))

	return writeJSONEnv(w, http.StatusOK, env)
}

func postImageCreate(eng *engine.Engine, version version.Version, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := r.ParseForm(); err != nil {
		return nil
	}
	authEncoded := r.Header.Get("X-Registry-Auth")
	authConfig := &cliconfig.AuthConfig{}
	if authEncoded != "" {
		authJson := base64.NewDecoder(base64.URLEncoding, strings.NewReader(authEncoded))
		if err := json.NewDecoder(authJson).Decode(authConfig); err != nil {
			// for a pull it is not an error if no auth was given
			// to increase compatibility with the existing api it is defaulting to be empty
			authConfig = &cliconfig.AuthConfig{}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	glog.V(1).Infof("Image name is %s", r.Form.Get("imageName"))
	job := eng.Job("pull", r.Form.Get("imageName"))

	output := ioutils.NewWriteFlusher(w)
	metaHeaders := map[string][]string{}
	for k, v := range r.Header {
		if strings.HasPrefix(k, "X-Meta-") {
			metaHeaders[k] = v
		}
	}
	job.Stdout.Add(output)
	imagePullConfig := &types.ImagePullConfig{
		MetaHeaders: metaHeaders,
		AuthConfig:  authConfig,
	}
	job.SetenvJson("ImagePullConfig", imagePullConfig)
	if err := job.Run(); err != nil {
		sf := streamformatter.NewJSONStreamFormatter()
		output.Write(sf.FormatError(err))
	}
	return nil
}

func postImageBuild(eng *engine.Engine, version version.Version, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := r.ParseForm(); err != nil {
		return nil
	}
	w.Header().Set("Content-Type", "application/json")

	glog.V(1).Infof("Image name is %s", r.Form.Get("name"))
	job := eng.Job("build", r.Form.Get("name"))
	stdoutBuf := bytes.NewBuffer(nil)

	job.Stdout.Add(stdoutBuf)
	job.Stdin.Add(r.Body)
	output := ioutils.NewWriteFlusher(w)
	if err := job.Run(); err != nil {
		sf := streamformatter.NewJSONStreamFormatter()
		output.Write(sf.FormatError(err))
	}
	return nil
}

func postImagePush(eng *engine.Engine, version version.Version, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if vars == nil {
		return fmt.Errorf("Missing parameter")
	}

	metaHeaders := map[string][]string{}
	for k, v := range r.Header {
		if strings.HasPrefix(k, "X-Meta-") {
			metaHeaders[k] = v
		}
	}
	if err := parseForm(r); err != nil {
		return err
	}
	authConfig := &cliconfig.AuthConfig{}
	output := ioutils.NewWriteFlusher(w)

	authEncoded := r.Header.Get("X-Registry-Auth")
	if authEncoded != "" {
		// the new format is to handle the authConfig as a header
		authJson := base64.NewDecoder(base64.URLEncoding, strings.NewReader(authEncoded))
		if err := json.NewDecoder(authJson).Decode(authConfig); err != nil {
			// to increase compatibility to existing api it is defaulting to be empty
			authConfig = &cliconfig.AuthConfig{}
		}
	} else {
		// the old format is supported for compatibility if there was no authConfig header
		if err := json.NewDecoder(r.Body).Decode(authConfig); err != nil {
			err = fmt.Errorf("Bad parameters and missing X-Registry-Auth: %v", err)
			sf := streamformatter.NewJSONStreamFormatter()
			output.Write(sf.FormatError(err))
			return nil
		}
	}

	imagePushConfig := &types.ImagePushConfig{
		MetaHeaders: metaHeaders,
		AuthConfig:  authConfig,
		Tag:         r.Form.Get("tag"),
	}

	w.Header().Set("Content-Type", "application/json")

	job := eng.Job("push", r.Form.Get("remote"))
	job.Stdout.Add(output)
	if err := job.SetenvJson("ImagePushConfig", imagePushConfig); err != nil {
		sf := streamformatter.NewJSONStreamFormatter()
		output.Write(sf.FormatError(err))
		return nil
	}
	if err := job.Run(); err != nil {
		sf := streamformatter.NewJSONStreamFormatter()
		output.Write(sf.FormatError(err))
	}

	return nil
}

func postTtyResize(eng *engine.Engine, version version.Version, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := r.ParseForm(); err != nil {
		return nil
	}
	job := eng.Job("tty", r.Form.Get("id"), r.Form.Get("tag"), r.Form.Get("h"), r.Form.Get("w"))
	if err := job.Run(); err != nil {
		return err
	}
	return nil
}

func postPodRemove(eng *engine.Engine, version version.Version, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := r.ParseForm(); err != nil {
		return nil
	}

	glog.V(1).Infof("Pod(%s) is process to be removed", r.Form.Get("podId"))
	job := eng.Job("podRm", r.Form.Get("podId"))
	stdoutBuf := bytes.NewBuffer(nil)

	job.Stdout.Add(stdoutBuf)

	if err := job.Run(); err != nil {
		return err
	}

	var (
		env             engine.Env
		dat             map[string]interface{}
		returnedJSONstr string
	)
	returnedJSONstr = engine.Tail(stdoutBuf, 1)
	if err := json.Unmarshal([]byte(returnedJSONstr), &dat); err != nil {
		return err
	}

	env.Set("ID", dat["ID"].(string))
	env.SetInt("Code", (int)(dat["Code"].(float64)))
	env.Set("Cause", dat["Cause"].(string))

	return writeJSONEnv(w, http.StatusOK, env)
}

func postImagesRemove(eng *engine.Engine, version version.Version, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := r.ParseForm(); err != nil {
		return nil
	}

	glog.V(1).Infof("Image(%s) is process to be removed", r.Form.Get("imageId"))
	var (
		force   string = "yes"
		noprune string = "yes"
	)
	if r.Form.Get("force") != "" {
		force = r.Form.Get("force")
	}
	if r.Form.Get("noprune") != "" {
		noprune = r.Form.Get("noprune")
	}
	job := eng.Job("imagesremove", r.Form.Get("imageId"), force, noprune)
	stdoutBuf := bytes.NewBuffer(nil)

	job.Stdout.Add(stdoutBuf)

	if err := job.Run(); err != nil {
		return err
	}

	str := engine.Tail(stdoutBuf, 1)
	type response struct {
		Images []string `json:"imagesList"`
	}
	var res response
	if err := json.Unmarshal([]byte(str), &res); err != nil {
		return err
	}
	var env engine.Env
	env.SetList("imagesList", res.Images)
	return writeJSONEnv(w, http.StatusOK, env)
}

func optionsHandler(eng *engine.Engine, version version.Version, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	w.WriteHeader(http.StatusOK)
	return nil
}
func writeCorsHeaders(w http.ResponseWriter, r *http.Request, corsHeaders string) {
	glog.V(1).Infof("CORS header is enabled and set to: %s", corsHeaders)
	w.Header().Add("Access-Control-Allow-Origin", corsHeaders)
	w.Header().Add("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept, X-Registry-Auth")
	w.Header().Add("Access-Control-Allow-Methods", "GET, POST, DELETE, PUT, OPTIONS")
}

func ping(eng *engine.Engine, version version.Version, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	_, err := w.Write([]byte{'O', 'K'})
	return err
}

func makeHttpHandler(eng *engine.Engine, logging bool, localMethod string, localRoute string, handlerFunc HttpApiFunc, corsHeaders string, dockerVersion version.Version) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// log the request
		glog.V(0).Infof("Calling %s %s", localMethod, localRoute)

		if logging {
			glog.V(1).Infof("%s %s", r.Method, r.RequestURI)
		}

		if strings.Contains(r.Header.Get("User-Agent"), "Docker-Client/") {
			userAgent := strings.Split(r.Header.Get("User-Agent"), "/")
			if len(userAgent) == 2 && !dockerVersion.Equal(version.Version(userAgent[1])) {
				glog.Warningf("client and server don't have the same version (client: %s, server: %s)", userAgent[1], dockerVersion)
			}
		}
		version := version.Version(mux.Vars(r)["version"])
		if version == "" {
			version = utils.APIVERSION
		}
		if corsHeaders != "" {
			writeCorsHeaders(w, r, corsHeaders)
		}

		if version.GreaterThan(utils.APIVERSION) {
			http.Error(w, fmt.Errorf("client and server don't have same version (client API version: %s, server API version: %s)", version, utils.APIVERSION).Error(), http.StatusNotFound)
			return
		}

		if err := handlerFunc(eng, version, w, r, mux.Vars(r)); err != nil {
			glog.Errorf("Handler for %s %s returned error: %s", localMethod, localRoute, err)
			httpError(w, err)
		}
	}
}

// Replicated from expvar.go as not public.
func expvarHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprintf(w, "{\n")
	first := true
	expvar.Do(func(kv expvar.KeyValue) {
		if !first {
			fmt.Fprintf(w, ",\n")
		}
		first = false
		fmt.Fprintf(w, "%q: %s", kv.Key, kv.Value)
	})
	fmt.Fprintf(w, "\n}\n")
}

func AttachProfiler(router *mux.Router) {
	router.HandleFunc("/debug/vars", expvarHandler)
	router.HandleFunc("/debug/pprof/", pprof.Index)
	router.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	router.HandleFunc("/debug/pprof/profile", pprof.Profile)
	router.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	router.HandleFunc("/debug/pprof/block", pprof.Handler("block").ServeHTTP)
	router.HandleFunc("/debug/pprof/heap", pprof.Handler("heap").ServeHTTP)
	router.HandleFunc("/debug/pprof/goroutine", pprof.Handler("goroutine").ServeHTTP)
	router.HandleFunc("/debug/pprof/threadcreate", pprof.Handler("threadcreate").ServeHTTP)
}

func createRouter(eng *engine.Engine, logging, enableCors bool, corsHeaders string, dockerVersion string) *mux.Router {
	r := mux.NewRouter()
	if os.Getenv("DEBUG") != "" {
		AttachProfiler(r)
	}
	m := map[string]map[string]HttpApiFunc{
		"GET": {
			"/info":           getInfo,
			"/pod/info":       getPodInfo,
			"/container/info": getContainerInfo,
			"/version":        getVersion,
			"/list":           getList,
			"/images/get":     getImages,
		},
		"POST": {
			"/auth":             postAuth,
			"/container/create": postContainerCreate,
			"/container/commit": postContainerCommit,
			"/container/rename": postContainerRename,
			"/image/create":     postImageCreate,
			"/image/build":      postImageBuild,
			"/image/push":       postImagePush,
			"/pod/create":       postPodCreate,
			"/pod/start":        postPodStart,
			"/pod/remove":       postPodRemove,
			"/pod/run":          postPodRun,
			"/pod/stop":         postStop,
			"/vm/create":        postVmCreate,
			"/vm/kill":          postVmKill,
			"/exec":             postExec,
			"/attach":           postAttach,
			"/tty/resize":       postTtyResize,
			"/images/remove":    postImagesRemove,
		},
		"DELETE": {},
		"OPTIONS": {
			"": optionsHandler,
		},
	}

	// If "api-cors-header" is not given, but "api-enable-cors" is true, we set cors to "*"
	// otherwise, all head values will be passed to HTTP handler
	if corsHeaders == "" && enableCors {
		corsHeaders = "*"
	}

	for method, routes := range m {
		for route, fct := range routes {
			glog.V(0).Infof("Registering %s, %s", method, route)
			// NOTE: scope issue, make sure the variables are local and won't be changed
			localRoute := route
			localFct := fct
			localMethod := method

			// build the handler function
			f := makeHttpHandler(eng, logging, localMethod, localRoute, localFct, corsHeaders, version.Version(dockerVersion))

			// add the new route
			if localRoute == "" {
				r.Methods(localMethod).HandlerFunc(f)
			} else {
				r.Path("/v{version:[0-9.]+}" + localRoute).Methods(localMethod).HandlerFunc(f)
				r.Path(localRoute).Methods(localMethod).HandlerFunc(f)
			}
		}
	}

	return r
}

// ServeRequest processes a single http request to the docker remote api.
// FIXME: refactor this to be part of Server and not require re-creating a new
// router each time. This requires first moving ListenAndServe into Server.
func ServeRequest(eng *engine.Engine, apiversion version.Version, w http.ResponseWriter, req *http.Request) {
	router := createRouter(eng, false, true, "", "")
	// Insert APIVERSION into the request as a convenience
	req.URL.Path = fmt.Sprintf("/v%s%s", apiversion, req.URL.Path)
	router.ServeHTTP(w, req)
}

func setupTls(cert, key, ca string, l net.Listener) (net.Listener, error) {
	tlsCert, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("Could not load X509 key pair (%s, %s): %v", cert, key, err)
		}
		return nil, fmt.Errorf("Error reading X509 key pair (%s, %s): %q. Make sure the key is encrypted.",
			cert, key, err)
	}
	tlsConfig := &tls.Config{
		NextProtos:   []string{"http/1.1"},
		Certificates: []tls.Certificate{tlsCert},
		// Avoid fallback on insecure SSL protocols
		MinVersion: tls.VersionTLS10,
	}

	if ca != "" {
		certPool := x509.NewCertPool()
		file, err := ioutil.ReadFile(ca)
		if err != nil {
			return nil, fmt.Errorf("Could not read CA certificate: %v", err)
		}
		certPool.AppendCertsFromPEM(file)
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		tlsConfig.ClientCAs = certPool
	}

	return tls.NewListener(l, tlsConfig), nil
}

func newListener(proto, addr string, bufferRequests bool) (net.Listener, error) {
	if bufferRequests {
		//		return listenbuffer.NewListenBuffer(proto, addr, activationLock)
	}

	return net.Listen(proto, addr)
}

func setSocketGroup(addr, group string) error {
	return nil
}

func allocateDaemonPort(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}

	intPort, err := strconv.Atoi(port)
	if err != nil {
		return err
	}

	var hostIPs []net.IP
	if parsedIP := net.ParseIP(host); parsedIP != nil {
		hostIPs = append(hostIPs, parsedIP)
	} else if hostIPs, err = net.LookupIP(host); err != nil {
		return fmt.Errorf("failed to lookup %s address in host specification", host)
	}

	for _, hostIP := range hostIPs {
		if _, err := portallocator.RequestPort(hostIP, "tcp", intPort); err != nil {
			return fmt.Errorf("failed to allocate daemon listening port %d (err: %v)", intPort, err)
		}
	}
	return nil
}

func setupTcpHttp(addr string, job *engine.Job) (*HttpServer, error) {
	r := createRouter(job.Eng, job.GetenvBool("Logging"), job.GetenvBool("EnableCors"), job.Getenv("CorsHeaders"), job.Getenv("Version"))

	l, err := newListener("tcp", addr, job.GetenvBool("BufferRequests"))
	if err != nil {
		return nil, err
	}
	if err := allocateDaemonPort(addr); err != nil {
		return nil, err
	}

	return &HttpServer{&http.Server{Addr: addr, Handler: r}, l}, nil
}

type Server interface {
	Serve() error
	Close() error
}

// ServeApi loops through all of the protocols sent in to docker and spawns
// off a go routine to setup a serving http.Server for each.
func ServeApi(job *engine.Job) error {
	if len(job.Args) == 0 {
		return fmt.Errorf("usage: %s PROTO://ADDR [PROTO://ADDR ...]", job.Name)
	}
	var (
		protoAddrs = job.Args
		chErrors   = make(chan error, len(protoAddrs))
	)
	activationLock = make(chan struct{})

	for _, protoAddr := range protoAddrs {
		protoAddrParts := strings.SplitN(protoAddr, "://", 2)
		if len(protoAddrParts) != 2 {
			return fmt.Errorf("usage: %s PROTO://ADDR [PROTO://ADDR ...]", job.Name)
		}
		go func() {
			glog.V(0).Infof("Listening for HTTP on %s (%s)", protoAddrParts[0], protoAddrParts[1])
			srv, err := NewServer(protoAddrParts[0], protoAddrParts[1], job)
			if err != nil {
				chErrors <- err
				return
			}
			job.Eng.OnShutdown(func() {
				if err := srv.Close(); err != nil {
					glog.Errorf("%v", err)
				}
			})
			if err = srv.Serve(); err != nil && strings.Contains(err.Error(), "use of closed network connection") {
				err = nil
			}
			chErrors <- err
		}()
	}

	for i := 0; i < len(protoAddrs); i++ {
		err := <-chErrors
		if err != nil {
			return err
		}
	}

	return nil
}

// NewServer sets up the required Server and does protocol specific checking.
func NewServer(proto, addr string, job *engine.Job) (Server, error) {
	// Basic error and sanity checking
	switch proto {
	case "tcp":
		return setupTcpHttp(addr, job)
	case "unix":
		return setupUnixHttp(addr, job)
	default:
		return nil, fmt.Errorf("Invalid protocol format.")
	}

	return nil, fmt.Errorf("Error.")
}

func setupUnixHttp(addr string, job *engine.Job) (*HttpServer, error) {
	r := createRouter(job.Eng, job.GetenvBool("Logging"), job.GetenvBool("EnableCors"), job.Getenv("CorsHeaders"), job.Getenv("Version"))

	if err := syscall.Unlink(addr); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	mask := syscall.Umask(0777)
	defer syscall.Umask(mask)

	l, err := newListener("unix", addr, job.GetenvBool("BufferRequests"))
	if err != nil {
		return nil, err
	}

	if err := setSocketGroup(addr, job.Getenv("SocketGroup")); err != nil {
		return nil, err
	}

	if err := os.Chmod(addr, 0777); err != nil {
		return nil, err
	}

	return &HttpServer{&http.Server{Addr: addr, Handler: r}, l}, nil
}

// Called through eng.Job("acceptconnections")
func AcceptConnections(job *engine.Job) error {
	// close the lock so the listeners start accepting connections
	if activationLock != nil {
		close(activationLock)
	}

	return nil
}
