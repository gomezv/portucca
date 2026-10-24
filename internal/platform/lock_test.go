package platform

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExclusiveLockBlocks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ports.json.lock")
	unlock, err := Lock(path, time.Second, false)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	_, err = Lock(path, 50*time.Millisecond, false)
	if !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("err = %v, want ErrLockTimeout", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("err = %q, want message containing lock path %q", err, path)
	}
}

func TestLockForcesMode600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ports.json.lock")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	unlock, err := Lock(path, time.Second, false)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %04o, want 0600", info.Mode().Perm())
	}
}

func TestSharedLocksDoNotBlockEachOther(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ports.json.lock")
	unlock1, err := Lock(path, time.Second, true)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock1()
	unlock2, err := Lock(path, 100*time.Millisecond, true)
	if err != nil {
		t.Fatalf("second shared lock: %v", err)
	}
	defer unlock2()
}
