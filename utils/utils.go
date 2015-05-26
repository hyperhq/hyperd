package utils

import (
	"os"
	"io"
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"encoding/base64"
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

	req,_ := http.NewRequest("GET", uri, nil);
	req.Header.Set("Range", "bytes=" + strconv.FormatInt(stat.Size(), 10) + "-")
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
