package probe

import (
	"context"
	"os"
	"os/exec"
	"time"

	"github.com/jnatkins/agentctl/internal/catalog"
	"github.com/jnatkins/agentctl/internal/pathutil"
	"github.com/jnatkins/agentctl/internal/target"
)

type Result struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Status   string `json:"status"`
	Evidence string `json:"evidence,omitempty"`
}

type Options struct {
	Selector target.Selector
	Live     bool
}

func Run(c *catalog.Catalog, opts Options) []Result {
	var results []Result
	for _, p := range c.CredentialProbes {
		if p.Status == "disabled" || !target.Matches(p.Targets, c, opts.Selector) {
			continue
		}
		results = append(results, runOne(p, opts.Live))
	}
	return results
}

func runOne(p catalog.CredentialProbe, live bool) Result {
	switch p.Type {
	case "fake", "":
		return Result{ID: p.ID, Type: p.Type, Status: "ok", Evidence: "fake probe"}
	case "file_exists", "path_exists":
		path := pathutil.Expand(p.Path)
		if _, err := os.Stat(path); err != nil {
			return Result{ID: p.ID, Type: p.Type, Status: "fail", Evidence: err.Error()}
		}
		return Result{ID: p.ID, Type: p.Type, Status: "ok", Evidence: path}
	case "command":
		if !live {
			return Result{ID: p.ID, Type: p.Type, Status: "skipped", Evidence: "command probes require --live-probes"}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, p.Command, p.Args...)
		out, err := cmd.CombinedOutput()
		if ctx.Err() == context.DeadlineExceeded {
			return Result{ID: p.ID, Type: p.Type, Status: "fail", Evidence: "timeout"}
		}
		if err != nil {
			return Result{ID: p.ID, Type: p.Type, Status: "fail", Evidence: string(out)}
		}
		return Result{ID: p.ID, Type: p.Type, Status: "ok", Evidence: string(out)}
	default:
		return Result{ID: p.ID, Type: p.Type, Status: "skipped", Evidence: "probe type is check-only placeholder"}
	}
}
