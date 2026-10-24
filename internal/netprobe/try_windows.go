//go:build windows

package netprobe

import (
	"errors"
	"net"
	"syscall"
)

const (
	wsaEAFNOSUPPORT  = syscall.Errno(10047)
	wsaEADDRNOTAVAIL = syscall.Errno(10049)
)

func tryListen(network, addr string) bool {
	ln, err := net.Listen(network, addr)
	if err == nil {
		ln.Close()
		return true
	}
	if errors.Is(err, wsaEAFNOSUPPORT) || errors.Is(err, wsaEADDRNOTAVAIL) ||
		errors.Is(err, syscall.EAFNOSUPPORT) || errors.Is(err, syscall.EADDRNOTAVAIL) {
		return true
	}
	return false
}
