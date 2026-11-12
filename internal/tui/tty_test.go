package tui

import (
	"os"
	"testing"
)

func TestIsTerminal(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "tty")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if IsTerminal(f) {
		t.Fatal("regular file reported as a terminal")
	}
	if IsTerminal(nil) {
		t.Fatal("nil file reported as a terminal")
	}
	devnull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devnull.Close()
	if IsTerminal(devnull) {
		t.Fatal("/dev/null reported as a terminal")
	}
}
