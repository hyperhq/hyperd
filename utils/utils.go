package utils

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	GITCOMMIT string = "0"
	VERSION   string

	IAMSTATIC string = "true"
	INITSHA1  string = ""
	INITPATH  string = ""

	HYPER_ROOT   string
	HYPER_FILE   string
	HYPER_DAEMON interface{}
)

const (
	APIVERSION = "1.17"
)

func MatchesContentType(contentType, expectedType string) bool {
	mimetype, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		fmt.Printf("Error parsing media type: %s error: %v", contentType, err)
	}
	return err == nil && mimetype == expectedType
}

func UriReader(uri string) (io.ReadCloser, error) {
	if strings.HasPrefix(uri, "http:") || strings.HasPrefix(uri, "https:") {
		req, _ := http.NewRequest("GET", uri, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		return resp.Body, nil
	} else if strings.HasPrefix(uri, "file://") {
		src := strings.TrimPrefix(uri, "file://")
		f, err := os.Open(src)
		if err != nil {
			return nil, err
		}
		return f, nil
	}
	return nil, fmt.Errorf("Unsupported URI: %s", uri)
}

// FormatMountLabel returns a string to be used by the mount command.
// The format of this string will be used to alter the labeling of the mountpoint.
// The string returned is suitable to be used as the options field of the mount command.
// If you need to have additional mount point options, you can pass them in as
// the first parameter.  Second parameter is the label that you wish to apply
// to all content in the mount point.
func FormatMountLabel(src, mountLabel string) string {
	if mountLabel != "" {
		switch src {
		case "":
			src = fmt.Sprintf("context=%q", mountLabel)
		default:
			src = fmt.Sprintf("%s,context=%q", src, mountLabel)
		}
	}
	return src
}

func PermInt(str string) int {
	var res = 0
	if str[0] == '0' {
		if len(str) == 1 {
			res = 0
		} else if str[1] == 'x' {
			// this is hex number
			i64, err := strconv.ParseInt(str[2:], 16, 0)
			if err == nil {
				res = int(i64)
			}
		} else {
			// this is a octal number
			i64, err := strconv.ParseInt(str[2:], 8, 0)
			if err == nil {
				res = int(i64)
			}
		}
	} else {
		res, _ = strconv.Atoi(str)
	}
	if res > 511 {
		res = 511
	}
	return res
}

func UidInt(str string) int {
	switch str {
	case "", "root":
		return 0
	default:
		i, err := strconv.Atoi(str)
		if err != nil {
			return 0
		}
		return i
	}
}

func RandStr(strSize int, randType string) string {
	var dictionary string
	if randType == "alphanum" {
		dictionary = "0123456789abcdefghijklmnopqrstuvwxyz"
	}

	if randType == "alpha" {
		dictionary = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	}

	if randType == "number" {
		dictionary = "0123456789"
	}

	var bytes = make([]byte, strSize)
	rand.Read(bytes)
	for k, v := range bytes {
		bytes[k] = dictionary[v%byte(len(dictionary))]
	}
	return string(bytes)
}

func JSONMarshal(v interface{}, safeEncoding bool) ([]byte, error) {
	b, err := json.Marshal(v)

	if safeEncoding {
		b = bytes.Replace(b, []byte("\\u003c"), []byte("<"), -1)
		b = bytes.Replace(b, []byte("\\u003e"), []byte(">"), -1)
		b = bytes.Replace(b, []byte("\\u0026"), []byte("&"), -1)
	}
	return b, err
}

func SetDaemon(d interface{}) {
	HYPER_DAEMON = d
}

func GetHostIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}

	for _, address := range addrs {
		// check the address type and if it is not a loopback the display it
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}

func ParseTimeString(str string) (int64, error) {
	t := time.Date(0, 0, 0, 0, 0, 0, 0, time.Local)
	if str == "" {
		return t.Unix(), nil
	}

	layout := "2006-01-02T15:04:05Z"
	t, err := time.Parse(layout, str)
	if err != nil {
		return t.Unix(), err
	}

	return t.Unix(), nil
}
