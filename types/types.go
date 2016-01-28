package types

import (
	apitypes "github.com/docker/engine-api/types"
)

type ImagePushConfig struct {
	MetaHeaders map[string][]string
	AuthConfig  *apitypes.AuthConfig
	Tag         string
}

type ImagePullConfig struct {
	MetaHeaders map[string][]string
	AuthConfig  *apitypes.AuthConfig
}
