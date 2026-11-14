//go:build !windows

package raknet

import (
	"errors"
	"syscall"
)

// isOversizedUDPReadError recognises EMSGSIZE, including errors wrapped by
// net.OpError and os.SyscallError (read/recvmsg). It may report a truncated
// datagram or an asynchronous path MTU error; neither should abort a handshake.
func isOversizedUDPReadError(err error) bool {
	return errors.Is(err, syscall.EMSGSIZE)
}
