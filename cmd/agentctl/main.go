package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jnatkins/agentctl/internal/catalog"
	"github.com/jnatkins/agentctl/internal/pathutil"
	"github.com/jnatkins/agentctl/internal/plan"
	"github.com/jnatkins/agentctl/internal/probe"
	"github.com/jnatkins/agentctl/internal/remote"
	"github.com/jnatkins/agentctl/internal/render"
	"github.com/jnatkins/agentctl/internal/target"
)

const version = "0.1.0"

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

type globalOptions struct {
	catalogs     stringList
	host         string
	targetGroups stringList
	format       string
	liveProbes   bool
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "agentctl:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage(os.Stdout)
		return nil
	}
	if args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		usage(os.Stdout)
		return nil
	}
	if args[0] == "--version" || args[0] == "version" {
		fmt.Println(version)
		return nil
	}
	cmd := args[0]
	switch cmd {
	case "check":
		return runCheck(args[1:])
	case "plan":
		return runPlan(args[1:])
	case "render":
		return runRender(args[1:])
	case "apply":
		return runApply(args[1:])
	case "remote":
		return runRemote(args[1:])
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func globalFlagSet(name string, opts *globalOptions) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.Var(&opts.catalogs, "f", "TOML catalog path; repeatable")
	fs.Var(&opts.catalogs, "catalog", "TOML catalog path; repeatable")
	fs.StringVar(&opts.host, "host", "", "host id, hostname, or alias to target")
	fs.Var(&opts.targetGroups, "target-group", "target group to target; repeatable")
	fs.StringVar(&opts.format, "format", "text", "output format: text or json")
	fs.BoolVar(&opts.liveProbes, "live-probes", false, "run command probes; default skips them")
	return fs
}

func runCheck(args []string) error {
	var opts globalOptions
	fs := globalFlagSet("check", &opts)
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, warnings, validation, err := loadAndValidate(opts)
	if err != nil {
		return err
	}
	results := probe.Run(c, probe.Options{Selector: selector(opts), Live: opts.liveProbes})
	if opts.format == "json" {
		return printJSON(map[string]any{"warnings": warnings, "validation": validation, "probes": results})
	}
	for _, w := range warnings {
		fmt.Println("warning:", w)
	}
	printDiagnostics(validation.Diagnostics)
	for _, p := range results {
		fmt.Printf("probe %-24s %-8s %s\n", p.ID, p.Status, oneLine(p.Evidence))
	}
	if validation.HasErrors() {
		return errors.New("catalog validation failed")
	}
	for _, p := range results {
		if p.Status == "fail" {
			return errors.New("one or more probes failed")
		}
	}
	fmt.Println("ok")
	return nil
}

func runPlan(args []string) error {
	var opts globalOptions
	fs := globalFlagSet("plan", &opts)
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, warnings, validation, err := loadAndValidate(opts)
	if err != nil {
		return err
	}
	if validation.HasErrors() {
		printDiagnostics(validation.Diagnostics)
		return errors.New("catalog validation failed")
	}
	p, err := plan.Build(c, selector(opts))
	if err != nil {
		return err
	}
	if opts.format == "json" {
		return printJSON(map[string]any{"warnings": warnings, "plan": p})
	}
	for _, w := range warnings {
		fmt.Println("warning:", w)
	}
	printChanges(p.Changes)
	return nil
}

func runRender(args []string) error {
	var opts globalOptions
	out := ""
	fs := globalFlagSet("render", &opts)
	fs.StringVar(&out, "out", "", "output directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if out == "" {
		out = filepath.Join(".", ".agentctl", "rendered")
	}
	c, warnings, validation, err := loadAndValidate(opts)
	if err != nil {
		return err
	}
	if validation.HasErrors() {
		printDiagnostics(validation.Diagnostics)
		return errors.New("catalog validation failed")
	}
	files, err := render.All(c, render.Options{Selector: selector(opts), OutputDir: out})
	if err != nil {
		return err
	}
	for _, f := range files {
		if err := os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(f.Path, f.Content, os.FileMode(f.Mode)); err != nil {
			return err
		}
	}
	if opts.format == "json" {
		return printJSON(map[string]any{"warnings": warnings, "files": files})
	}
	for _, w := range warnings {
		fmt.Println("warning:", w)
	}
	for _, f := range files {
		fmt.Printf("rendered %s (%s)\n", f.Path, f.Reason)
	}
	return nil
}

func runApply(args []string) error {
	var opts globalOptions
	fs := globalFlagSet("apply", &opts)
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, warnings, validation, err := loadAndValidate(opts)
	if err != nil {
		return err
	}
	if validation.HasErrors() {
		printDiagnostics(validation.Diagnostics)
		return errors.New("catalog validation failed")
	}
	changes, err := plan.Apply(c, selector(opts))
	if opts.format == "json" {
		return printJSON(map[string]any{"warnings": warnings, "changes": changes, "error": errorString(err)})
	}
	for _, w := range warnings {
		fmt.Println("warning:", w)
	}
	printChanges(changes)
	return err
}

func runRemote(args []string) error {
	var opts globalOptions
	remotePath := ""
	fs := globalFlagSet("remote", &opts)
	fs.StringVar(&remotePath, "remote-path", "", "path to cd into before running agentctl remotely")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) < 2 {
		return errors.New("usage: agentctl remote [flags] <host|group> <check|plan|apply|render> [args...]")
	}
	c, _, validation, err := loadAndValidate(opts)
	if err != nil {
		return err
	}
	if validation.HasErrors() {
		printDiagnostics(validation.Diagnostics)
		return errors.New("catalog validation failed")
	}
	return remote.Run(c, remote.Options{
		Selector:   rest[0],
		Subcommand: rest[1],
		Args:       rest[2:],
		RemotePath: remotePath,
	})
}

func loadAndValidate(opts globalOptions) (*catalog.Catalog, []string, catalog.ValidationResult, error) {
	paths := []string(opts.catalogs)
	if len(paths) == 0 {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, nil, catalog.ValidationResult{}, err
		}
		paths, err = catalog.DiscoverCatalogs(cwd)
		if err != nil {
			return nil, nil, catalog.ValidationResult{}, err
		}
	}
	c, warnings, err := catalog.Load(paths)
	if err != nil {
		return nil, warnings, catalog.ValidationResult{}, err
	}
	validation := catalog.Validate(c)
	return c, warnings, validation, nil
}

func selector(opts globalOptions) target.Selector {
	return target.Selector{Host: opts.host, TargetGroups: []string(opts.targetGroups)}
}

func printDiagnostics(diags []catalog.Diagnostic) {
	for _, d := range diags {
		id := d.Resource
		if d.ID != "" {
			id += "." + d.ID
		}
		fmt.Printf("%s %-28s %s\n", d.Severity, id, d.Message)
	}
}

func printChanges(changes []plan.Change) {
	for _, c := range changes {
		target := c.Resource
		if c.ID != "" {
			target += "." + c.ID
		}
		if c.Path != "" {
			target += " " + pathutil.Expand(c.Path)
		}
		fmt.Printf("%-8s %-60s risk=%s %s\n", c.Action, target, c.Risk, oneLine(c.Reason))
	}
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func oneLine(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 160 {
		return s[:157] + "..."
	}
	return s
}

func usage(out *os.File) {
	fmt.Fprintln(out, `agentctl - harness-neutral local agent-box desired state

Usage:
  agentctl check  [flags]
  agentctl plan   [flags]
  agentctl render [flags] [--out DIR]
  agentctl apply  [flags]
  agentctl remote [flags] <host|group> <check|plan|apply|render> [args...]

Common flags:
  -f, --catalog PATH      TOML catalog path; repeatable
      --host ID           target one host id, hostname, or alias
      --target-group NAME target one group; repeatable
      --format text|json  output format
      --live-probes       run command probes during check`)
}
