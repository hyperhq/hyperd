package api

import "errors"

var (
	ErrConnectionRefused = errors.New("Cannot connect to the Hyper daemon. Is 'hyperd' running on this host?")
)
