package cli

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gomezv/portucca/internal/application"
	"github.com/gomezv/portucca/internal/domain"
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

func TestJSONTokenAsFlagValueDoesNotEnableJSON(t *testing.T) {
	var out, errb bytes.Buffer
	code := Run([]string{"ensure", "--worktree", "--json"}, &out, &errb)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want human output only on stderr", out.String())
	}
	if !strings.Contains(errb.String(), "error:") {
		t.Fatalf("stderr = %q, want a human error", errb.String())
	}
}

func TestJSONEqualsTrueEmitsJSON(t *testing.T) {
	var out, errb bytes.Buffer
	code := Run([]string{"version", "--json=true"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	var env struct {
		SchemaVersion int    `json:"schemaVersion"`
		Command       string `json:"command"`
		OK            bool   `json:"ok"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("out = %q: %v", out.String(), err)
	}
	if env.SchemaVersion != 1 || env.Command != "version" || !env.OK {
		t.Fatalf("envelope = %+v, want a version success envelope", env)
	}
	if errb.Len() != 0 {
		t.Fatalf("stderr = %q, want empty with --json", errb.String())
	}
}

func TestVersionJSONEnvelope(t *testing.T) {
	var out, errb bytes.Buffer
	code := Run([]string{"version", "--json"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	var env struct {
		SchemaVersion int    `json:"schemaVersion"`
		Command       string `json:"command"`
		OK            bool   `json:"ok"`
		Data          struct {
			Version string `json:"version"`
			Commit  string `json:"commit"`
			OS      string `json:"os"`
			Arch    string `json:"arch"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("out = %q: %v", out.String(), err)
	}
	if env.SchemaVersion != 1 || env.Command != "version" || !env.OK {
		t.Fatalf("envelope = %+v, want a version success envelope", env)
	}
	if env.Data.Version == "" || env.Data.Commit == "" || env.Data.OS == "" || env.Data.Arch == "" {
		t.Fatalf("data = %+v, want version, commit, os, and arch", env.Data)
	}
	if errb.Len() != 0 {
		t.Fatalf("stderr = %q, want empty with --json", errb.String())
	}
}

func TestVersionShortcutJSON(t *testing.T) {
	var out, errb bytes.Buffer
	code := Run([]string{"--version", "--json"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !json.Valid(out.Bytes()) || !strings.Contains(out.String(), `"schemaVersion": 1`) {
		t.Fatalf("out = %q, want a version JSON envelope", out.String())
	}
}

func TestVersionUsageErrors(t *testing.T) {
	cases := [][]string{
		{"version", "--bogus"},
		{"version", "extra"},
		{"--version", "--bogus"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out, errb bytes.Buffer
			code := Run(args, &out, &errb)
			if code != 2 {
				t.Fatalf("exit = %d, want 2", code)
			}
			if errb.Len() == 0 {
				t.Fatal("stderr is empty, want a usage error")
			}
		})
	}
}

func TestEnsureRejectsUnknownFlag(t *testing.T) {
	var out, errb bytes.Buffer
	code := Run([]string{"ensure", "--bogus"}, &out, &errb)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "bogus") {
		t.Fatalf("stderr = %q, want the unknown flag named", errb.String())
	}
}

func TestInitRequiresProjectAndService(t *testing.T) {
	var out, errb bytes.Buffer
	code := Run([]string{"init"}, &out, &errb)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "--project") {
		t.Fatalf("stderr = %q, want the missing flag message", errb.String())
	}
}

func TestInitMissingFlagsWithNoHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	var out, errb bytes.Buffer
	code := Run([]string{"init"}, &out, &errb)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "--project") {
		t.Fatalf("stderr = %q, want the missing flag message", errb.String())
	}
}

func TestHelpExitsZero(t *testing.T) {
	var out, errb bytes.Buffer
	code := Run([]string{"--help"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("out = %q, want usage", out.String())
	}
	if !strings.Contains(out.String(), "Commands:") {
		t.Fatalf("out = %q, want command list", out.String())
	}
}

func TestCommandHelp(t *testing.T) {
	commands := []string{"init", "ensure", "setup", "reallocate", "prune", "doctor", "env", "list", "run", "version", "help"}
	for _, cmd := range commands {
		t.Run(cmd, func(t *testing.T) {
			var out, errb bytes.Buffer
			if code := Run([]string{cmd, "--help"}, &out, &errb); code != 0 {
				t.Fatalf("exit = %d, stderr = %s", code, errb.String())
			}
			text := out.String()
			for _, want := range []string{"portucca " + cmd, "Usage:", "Examples:"} {
				if !strings.Contains(text, want) {
					t.Fatalf("stdout = %q, want %q", text, want)
				}
			}
			if errb.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", errb.String())
			}
		})
	}
}

func TestCommandHelpSpecifics(t *testing.T) {
	cases := []struct {
		cmd  string
		want []string
	}{
		{"init", []string{"--project", "--service", "--preset", "node-vite-nest", "next", "--force"}},
		{"list", []string{"--all", "--project"}},
		{"run", []string{"--service", "exec", "--json is not supported"}},
		{"doctor", []string{"Exits 1"}},
	}
	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			var out, errb bytes.Buffer
			if code := Run([]string{tc.cmd, "--help"}, &out, &errb); code != 0 {
				t.Fatalf("exit = %d", code)
			}
			for _, want := range tc.want {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("stdout = %q, want %q", out.String(), want)
				}
			}
		})
	}
}

func TestHelpCommand(t *testing.T) {
	var out, errb bytes.Buffer
	if code := Run([]string{"help", "run"}, &out, &errb); code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "portucca run") {
		t.Fatalf("stdout = %q, want run help", out.String())
	}
	if errb.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", errb.String())
	}
}

func TestHelpUnknownCommand(t *testing.T) {
	var out, errb bytes.Buffer
	if code := Run([]string{"help", "nope"}, &out, &errb); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "unknown command") {
		t.Fatalf("stderr = %q, want unknown command", errb.String())
	}
}

func TestUnknownCommandWithNoHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	var out, errb bytes.Buffer
	code := Run([]string{"nope"}, &out, &errb)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "unknown command") {
		t.Fatalf("stderr = %q, want unknown command", errb.String())
	}
}

func TestPrintEnsureSortsServices(t *testing.T) {
	v := application.EnsureResult{Services: map[domain.ServiceName]domain.ServiceClaim{
		"web": {Port: 18080},
		"api": {Port: 18090},
	}}
	for i := 0; i < 20; i++ {
		var out bytes.Buffer
		printEnsure(&out, v)
		if strings.Index(out.String(), "api:") > strings.Index(out.String(), "web:") {
			t.Fatalf("output = %q, want api before web", out.String())
		}
	}
}

func TestHasSeparator(t *testing.T) {
	if !hasSeparator([]string{"--service", "web", "--", "npm", "run", "dev"}) {
		t.Fatal("expected separator")
	}
	if hasSeparator([]string{"npm", "run", "dev"}) {
		t.Fatal("unexpected separator")
	}
}

func TestRunHelp(t *testing.T) {
	var out, errb bytes.Buffer
	if code := Run([]string{"run", "--help"}, &out, &errb); code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "portucca run") {
		t.Fatalf("stdout = %q, want run usage", out.String())
	}
	if !strings.Contains(out.String(), "--service") || !strings.Contains(out.String(), "exec") {
		t.Fatalf("stdout = %q, want run flags and exec description", out.String())
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
