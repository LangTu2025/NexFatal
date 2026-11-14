//go:build windows

package raknet

import (
	"errors"
	"syscall"
)

const wsaeMsgSize = syscall.Errno(10040) // WSAEMSGSIZE

// isOversizedUDPReadError recognises Winsock's message-too-long error, including
// errors wrapped by net.OpError and os.SyscallError. Discard the truncated
// datagram and continue receiving.
func isOversizedUDPReadError(err error) bool {
	return errors.Is(err, wsaeMsgSize)
}
