package netprobe

import (
	"net"
	"testing"

	"github.com/gomezv/portucca/internal/domain"
)

func TestIsFreeFalseWhileListening(t *testing.T) {
	ln, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := domain.Port(ln.Addr().(*net.TCPAddr).Port)
	if (Prober{}).IsFree(port) {
		t.Fatalf("IsFree(%d) = true while listening", port)
	}
}

func TestIsFreeTrueAfterClose(t *testing.T) {
	ln, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	port := domain.Port(ln.Addr().(*net.TCPAddr).Port)
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	if !(Prober{}).IsFree(port) {
		t.Fatalf("IsFree(%d) = false after close", port)
	}
}
