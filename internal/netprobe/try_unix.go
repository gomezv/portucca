//go:build unix

package netprobe

import (
	"errors"
	"net"
	"syscall"
)

func tryListen(network, addr string) bool {
	ln, err := net.Listen(network, addr)
	if err == nil {
		ln.Close()
		return true
	}
	if errors.Is(err, syscall.EAFNOSUPPORT) || errors.Is(err, syscall.EADDRNOTAVAIL) {
		return true
	}
	return false
}
