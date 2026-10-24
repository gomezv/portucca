package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/gomezv/portucca/internal/apperr"
	"github.com/gomezv/portucca/internal/application"
)

type envelope struct {
	SchemaVersion int           `json:"schemaVersion"`
	Command       string        `json:"command"`
	OK            bool          `json:"ok"`
	Data          any           `json:"data,omitempty"`
	Error         *apperr.Error `json:"error,omitempty"`
}

func writeJSON(w io.Writer, cmd string, data any, err error) {
	env := envelope{SchemaVersion: 1, Command: cmd, OK: err == nil, Data: data}
	if err != nil {
		ae, ok := apperr.As(err)
		if !ok {
			ae = apperr.New("internal", err.Error(), "")
		}
		env.Error = ae
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(env)
}

func writeHuman(w io.Writer, cmd string, data any, err error) {
	if err != nil {
		ae, ok := apperr.As(err)
		if !ok {
			fmt.Fprintf(w, "error: %v\n", err)
			return
		}
		fmt.Fprintf(w, "error: %s\n", ae.Message)
		if ae.Hint != "" {
			fmt.Fprintf(w, "hint: %s\n", ae.Hint)
		}
		return
	}
	switch v := data.(type) {
	case application.EnsureResult:
		printEnsure(w, v)
	case application.SetupResult:
		printEnsure(w, v.EnsureResult)
		fmt.Fprintf(w, "bootstrap: copied=%v symlinked=%v skipped=%v\n", v.Bootstrap.Copied, v.Bootstrap.Symlinked, v.Bootstrap.Skipped)
	case application.EnvResult:
		fmt.Fprintf(w, "project: %s\nworktree: %s (main=%v)\n", v.Project, v.WorktreePath, v.IsMain)
		for _, e := range v.Env {
			fmt.Fprintf(w, "%s %s=%s\n", e.File, e.Key, e.Value)
		}
	case application.ListResult:
		for _, c := range v.Claims {
			fmt.Fprintf(w, "%s %s live=%v %v\n", c.Project, c.Path, c.Live, c.Services)
		}
	case application.PruneResult:
		fmt.Fprintf(w, "removed: %d\n", len(v.Removed))
	case application.DoctorResult:
		fmt.Fprintf(w, "ok: %v errors=%d warnings=%d\n", v.OK, len(v.Errors), len(v.Warnings))
		for _, e := range v.Errors {
			fmt.Fprintf(w, "error: %s %s\n", e.Code, e.Message)
		}
		for _, e := range v.Warnings {
			fmt.Fprintf(w, "warning: %s %s\n", e.Code, e.Message)
		}
	case application.InitResult:
		fmt.Fprintf(w, "config: %s\nproject: %s\n", v.ConfigPath, v.Project)
		if v.Warning != "" {
			fmt.Fprintf(w, "warning: %s\n", v.Warning)
		}
	default:
		fmt.Fprintf(w, "%v\n", v)
	}
}

func printEnsure(w io.Writer, v application.EnsureResult) {
	fmt.Fprintf(w, "project: %s\nworktree: %s (main=%v) changed=%v\n", v.Project, v.WorktreePath, v.IsMain, v.Changed)
	for name, sc := range v.Services {
		fmt.Fprintf(w, "%s: %d adopted=%v\n", name, sc.Port, sc.Adopted)
	}
	for _, e := range v.Env {
		fmt.Fprintf(w, "%s %s=%s\n", e.File, e.Key, e.Value)
	}
}
