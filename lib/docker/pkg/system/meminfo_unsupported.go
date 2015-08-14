// +build !linux,!windows,!darwin

package system

func ReadMemInfo() (*MemInfo, error) {
	return nil, ErrNotSupportedPlatform
}
