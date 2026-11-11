package cli

import (
	"bytes"
	"encoding/json"
	"os"
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
		{"init", []string{"--project", "--service", "--preset", "node-vite-nest", "next", "--force", "--compose", "--exclude-service"}},
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

func TestHasRunSeparator(t *testing.T) {
	cases := []struct {
		name string
		args []string
		rest []string
		want bool
	}{
		{"before command", []string{"--service", "web", "--", "npm"}, []string{"npm"}, true},
		{"no separator", []string{"--service", "web", "npm"}, []string{"npm"}, false},
		{"trailing separator", []string{"echo", "--"}, []string{"echo", "--"}, false},
		{"only separator", []string{"--"}, []string{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasRunSeparator(tc.args, tc.rest); got != tc.want {
				t.Fatalf("hasRunSeparator(%v, %v) = %v, want %v", tc.args, tc.rest, got, tc.want)
			}
		})
	}
}

func TestRunRejectsTrailingSeparator(t *testing.T) {
	var out, errb bytes.Buffer
	if code := Run([]string{"run", "echo", "--"}, &out, &errb); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "run: missing -- before the command") {
		t.Fatalf("stderr = %q, want the missing separator message", errb.String())
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
	if !strings.Contains(out.String(), "--worktree") || !strings.Contains(out.String(), "current directory") {
		t.Fatalf("stdout = %q, want the --worktree CWD note", out.String())
	}
}

func TestInitScanRejectsPreset(t *testing.T) {
	dir := newGitWorktree(t)
	var out, errb bytes.Buffer
	code := Run([]string{
		"init", "--scan", "--preset", "generic", "--worktree", dir, "--json",
	}, &out, &errb)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(out.String(), `"code": "usage"`) {
		t.Fatalf("out = %q, want usage", out.String())
	}
}

func TestInitScanWritesConfig(t *testing.T) {
	dir := newGitWorktree(t)
	api := filepath.Join(dir, "apps", "api")
	if err := os.MkdirAll(api, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(api, ".env"), []byte("PORT=19004\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := Run([]string{"init", "--scan", "--worktree", dir, "--json"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d, out=%s err=%s", code, out.String(), errb.String())
	}
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Project  string `json:"project"`
			Services map[string]struct {
				MainPort  int `json:"mainPort"`
				StartPort int `json:"startPort"`
			} `json:"services"`
			Scan struct {
				Sources []application.ScanSource `json:"sources"`
			} `json:"scan"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("out = %q: %v", out.String(), err)
	}
	if !env.OK || env.Data.Services["api"].MainPort != 19004 || env.Data.Services["api"].StartPort != 19005 {
		t.Fatalf("data = %+v", env.Data)
	}
	if len(env.Data.Scan.Sources) == 0 {
		t.Fatalf("sources = %+v, want at least one", env.Data.Scan.Sources)
	}
	if _, err := os.Stat(filepath.Join(dir, "portucca.json")); err != nil {
		t.Fatal(err)
	}
}

func TestInitScanHumanOutput(t *testing.T) {
	dir := newGitWorktree(t)
	api := filepath.Join(dir, "apps", "api")
	if err := os.MkdirAll(api, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(api, ".env"), []byte("PORT=19004\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := Run([]string{"init", "--scan", "--worktree", dir}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d, out=%s err=%s", code, out.String(), errb.String())
	}
	got := out.String()
	for _, want := range []string{"env[0]=", "bootstrap.copyFromMain[0]=", "services.api.name="} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want %q", got, want)
		}
	}
}

func TestInitScanIncompleteJSON(t *testing.T) {
	dir := newGitWorktree(t)
	var out, errb bytes.Buffer
	code := Run([]string{"init", "--scan", "--worktree", dir, "--json"}, &out, &errb)
	if code != 1 {
		t.Fatalf("exit = %d, want 1; out=%s err=%s", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), `"code": "scan_incomplete"`) {
		t.Fatalf("out = %q, want scan_incomplete", out.String())
	}
	if !strings.Contains(out.String(), `"gaps"`) {
		t.Fatalf("out = %q, want gaps", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "portucca.json")); !os.IsNotExist(err) {
		t.Fatalf("config was written: %v", err)
	}
}

func TestInitScanHelp(t *testing.T) {
	var out, errb bytes.Buffer
	code := Run([]string{"help", "init"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	text := out.String()
	for _, want := range []string{"--scan", "--compose", "--exclude-service", "Compose JSON/YAML", "configured by hand", "does not reserve"} {
		if !strings.Contains(text, want) {
			t.Fatalf("help = %q, want %q", text, want)
		}
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

func TestInitComposeFlagValidation(t *testing.T) {
	dir := newGitWorktree(t)
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"compose requires scan", []string{"init", "--json", "--worktree", dir, "--compose"}, "--compose requires --scan"},
		{"exclude requires compose", []string{"init", "--json", "--worktree", dir, "--scan", "--exclude-service", "db"}, "--exclude-service requires --compose"},
		{"exclude with service", []string{"init", "--json", "--worktree", dir, "--scan", "--compose", "--exclude-service", "db", "--service", "web:8080:8081"}, "--exclude-service cannot be combined with --service"},
		{"invalid exclude", []string{"init", "--json", "--worktree", dir, "--scan", "--compose", "--exclude-service", "!!!"}, "invalid --exclude-service"},
		{"empty exclude", []string{"init", "--json", "--worktree", dir, "--scan", "--compose", "--exclude-service", ""}, "invalid --exclude-service"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			code := Run(tc.args, &out, &errb)
			if code != 2 {
				t.Fatalf("exit = %d, want 2", code)
			}
			if !strings.Contains(out.String(), `"code": "usage"`) || !strings.Contains(out.String(), tc.want) {
				t.Fatalf("out = %q, want usage %q", out.String(), tc.want)
			}
		})
	}
}

func TestInitScanComposeDiscoversServices(t *testing.T) {
	dir := newGitWorktree(t)
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services:\n  web:\n    ports:\n      - \"4000:4000\"\n  db:\n    ports:\n      - \"5432:5432\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := Run([]string{"init", "--scan", "--compose", "--exclude-service", "db", "--worktree", dir, "--json"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	var env struct {
		Data struct {
			Services map[string]struct {
				MainPort int `json:"mainPort"`
			} `json:"services"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Data.Services) != 1 {
		t.Fatalf("services = %+v, want only web", env.Data.Services)
	}
	if env.Data.Services["web"].MainPort != 4000 {
		t.Fatalf("services = %+v, want web 4000", env.Data.Services)
	}
}

func TestInitScanComposeExplicitWins(t *testing.T) {
	dir := newGitWorktree(t)
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services:\n  web:\n    ports:\n      - \"4000:4000\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := Run([]string{"init", "--scan", "--compose", "--service", "web:9000:9001", "--worktree", dir, "--json"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	var env struct {
		Data struct {
			Services map[string]struct {
				MainPort int `json:"mainPort"`
			} `json:"services"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Data.Services["web"].MainPort != 9000 {
		t.Fatalf("services = %+v, want explicit web 9000", env.Data.Services)
	}
}

func TestPruneHumanOutputReportsMains(t *testing.T) {
	dir := t.TempDir()
	reg := filepath.Join(dir, "ports.json")
	dead := filepath.Join(dir, "gone")
	raw := `{"version":2,"worktrees":[],"mains":[{"project":"app","path":"` + dead + `","ports":{"web":18080},"updatedAt":"2026-11-10T00:00:00Z"}]}`
	if err := os.WriteFile(reg, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := Run([]string{"prune", "--registry", reg}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d, out=%s err=%s", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "removed: 0 mains: 1") {
		t.Fatalf("output = %q, want removed and mains counts", out.String())
	}
}

func newGitWorktreeWithBranch(t *testing.T) (string, string) {
	t.Helper()
	main := newGitWorktree(t)
	if err := os.WriteFile(filepath.Join(main, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", main, "add", ".").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, out)
	}
	cmd := exec.Command("git", "-C", main, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-qm", "init")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, out)
	}
	wt := filepath.Join(t.TempDir(), "wt")
	cmd = exec.Command("git", "-C", main, "worktree", "add", "-q", wt, "-b", "feature")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v: %s", err, out)
	}
	return main, wt
}

func TestReleaseHumanOutputAndServiceFlag(t *testing.T) {
	_, wt := newGitWorktreeWithBranch(t)
	real, err := filepath.EvalSymlinks(wt)
	if err != nil {
		t.Fatal(err)
	}
	reg := filepath.Join(t.TempDir(), "ports.json")
	raw := `{"version":2,"worktrees":[{"project":"app","path":"` + real + `","services":{"web":{"port":18080},"api":{"port":18081}},"updatedAt":"2026-11-11T00:00:00Z"}],"mains":[]}`
	if err := os.WriteFile(reg, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := Run([]string{"release", "--worktree", wt, "--registry", reg, "--service", "web"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d, out=%s err=%s", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "released: web=18080") {
		t.Fatalf("output = %q", out.String())
	}
	data, err := os.ReadFile(reg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"web"`) || !strings.Contains(string(data), `"api"`) {
		t.Fatalf("registry = %s, want only api", data)
	}
}

func TestReleaseHelp(t *testing.T) {
	var out, errb bytes.Buffer
	if code := Run([]string{"help", "release"}, &out, &errb); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out.String(), "--service") || !strings.Contains(out.String(), "main_reserved") {
		t.Fatalf("help = %q", out.String())
	}
}
