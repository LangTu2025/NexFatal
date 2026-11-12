//go:build !windows

package raknet

import (
	"errors"
	"syscall"
)

// isOversizedUDPReadError reports whether recvmsg returned EMSGSIZE because
// the datagram did not fit the caller-provided buffer.
func isOversizedUDPReadError(err error) bool {
	if err == nil {
		return false
	}
	var errno syscall.Errno
	return errors.As(err, &errno) && errno == syscall.EMSGSIZE
}
