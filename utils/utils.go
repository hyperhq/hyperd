package utils

import (
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"strconv"
)

var (
	GITCOMMIT string = "0"
	VERSION   string = "0.1"

	IAMSTATIC string = "true"
	INITSHA1  string = ""
	INITPATH  string = ""
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

func DownloadFile(uri, target string) error {
	f, err := os.OpenFile(target, os.O_RDWR|os.O_CREATE, 0666)
	stat, err := f.Stat()
	if err != nil {
		return err
	}

	req, _ := http.NewRequest("GET", uri, nil)
	req.Header.Set("Range", "bytes="+strconv.FormatInt(stat.Size(), 10)+"-")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, resp.Body)
	if err != nil {
		return err
	}
	return nil
}

func Base64Decode(fileContent string) (string, error) {
	b64 := base64.NewEncoding("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/")
	decodeBytes, err := b64.DecodeString(fileContent)
	if err != nil {
		return "", err
	}
	return string(decodeBytes), nil
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
