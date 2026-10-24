package cli

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var out, errb bytes.Buffer
	code := Run([]string{"version"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "portucca") {
		t.Fatalf("output = %q, want it to contain portucca", out.String())
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var out, errb bytes.Buffer
	code := Run([]string{"nope"}, &out, &errb)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "unknown command") {
		t.Fatalf("stderr = %q, want unknown command", errb.String())
	}
}

func TestEnvelopeErrorShape(t *testing.T) {
	var out, errb bytes.Buffer
	code := Run([]string{"ensure", "--json", "--worktree", "/definitely/missing"}, &out, &errb)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(out.String(), `"schemaVersion": 1`) || !strings.Contains(out.String(), `"ok": false`) {
		t.Fatalf("out = %q", out.String())
	}
	if errb.Len() != 0 {
		t.Fatalf("stderr = %q, want empty with --json", errb.String())
	}
}

func TestListRejectsAllWithProject(t *testing.T) {
	var out, errb bytes.Buffer
	code := Run([]string{"list", "--all", "--project", "app"}, &out, &errb)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "mutually exclusive") {
		t.Fatalf("stderr = %q, want the mutual exclusion message", errb.String())
	}
}

func TestInitRejectsInvalidProjectValue(t *testing.T) {
	dir := newGitWorktree(t)
	var out, errb bytes.Buffer
	code := Run([]string{
		"init", "--json", "--worktree", dir,
		"--project", "Bad", "--service", "web:8080:8081",
	}, &out, &errb)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(out.String(), `"code": "usage"`) {
		t.Fatalf("out = %q, want a usage error envelope", out.String())
	}
	if errb.Len() != 0 {
		t.Fatalf("stderr = %q, want empty with --json", errb.String())
	}
}

func TestInitRejectsInvalidPreset(t *testing.T) {
	dir := newGitWorktree(t)
	var out, errb bytes.Buffer
	code := Run([]string{
		"init", "--json", "--worktree", dir,
		"--project", "app", "--preset", "nope", "--service", "web:8080:8081",
	}, &out, &errb)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(out.String(), `"code": "usage"`) {
		t.Fatalf("out = %q, want a usage error envelope", out.String())
	}
}

func TestDoctorExitsOneWhenDataIsNotOK(t *testing.T) {
	dir := newGitWorktree(t)
	registry := filepath.Join(t.TempDir(), "ports.json")
	var out, errb bytes.Buffer
	code := Run([]string{"doctor", "--json", "--worktree", dir, "--registry", registry}, &out, &errb)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	var env struct {
		SchemaVersion int  `json:"schemaVersion"`
		OK            bool `json:"ok"`
		Data          struct {
			OK bool `json:"ok"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("out = %q: %v", out.String(), err)
	}
	if env.SchemaVersion != 1 || !env.OK || env.Data.OK {
		t.Fatalf("envelope = %+v, want ok true and data.ok false", env)
	}
}

func TestListJSONSuccessEnvelope(t *testing.T) {
	dir := newGitWorktree(t)
	registry := filepath.Join(t.TempDir(), "ports.json")
	var out, errb bytes.Buffer
	code := Run([]string{"list", "--json", "--worktree", dir, "--registry", registry}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	var env struct {
		SchemaVersion int    `json:"schemaVersion"`
		Command       string `json:"command"`
		OK            bool   `json:"ok"`
		Data          struct {
			Claims []json.RawMessage `json:"claims"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("out = %q: %v", out.String(), err)
	}
	if env.SchemaVersion != 1 || env.Command != "list" || !env.OK || env.Data.Claims == nil {
		t.Fatalf("envelope = %+v, want a list success envelope with claims", env)
	}
	if errb.Len() != 0 {
		t.Fatalf("stderr = %q, want empty with --json", errb.String())
	}
}

func newGitWorktree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init", "-q", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	return dir
}
