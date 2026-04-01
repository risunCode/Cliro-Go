//go:build !windows

package platform

func ensureProtocolRegistered() (bool, error) {
	return false, nil
}
