package utils

import (
	"regexp"
	"strings"
)

const (
	Dns1123LabelFmt           = "[a-z0-9]([-a-z0-9]*[a-z0-9])?"
	dns1123LabelMaxLength int = 63
)

var (
	dns1123LabelRegexp = regexp.MustCompile("^" + Dns1123LabelFmt + "$")
)

func IsDNSLabel(value string) bool {
	return IsDNS1123Label(value)
}

// IsDNS1123Label tests for a string that conforms to the definition of a label in
// DNS (RFC 1123).
func IsDNS1123Label(value string) bool {
	return len(value) <= dns1123LabelMaxLength && dns1123LabelRegexp.MatchString(value)
}

func ParseImageRepoTag(image string) (repo, name, tag string) {
	all := strings.SplitN(image, ":", 2)

	tag = ""
	if len(all) > 1 {
		tag = all[1]
	}

	i := strings.LastIndex(all[0], "/")
	if i >= 0 {
		repo = all[0][:i]
		name = all[0][i+1:]
	} else {
		repo = ""
		name = all[0]
	}

	return repo, name, tag
}
