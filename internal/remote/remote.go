package remote

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/jnatkins/agentctl/internal/catalog"
	"github.com/jnatkins/agentctl/internal/target"
)

type Options struct {
	Selector   string
	Subcommand string
	Args       []string
	RemotePath string
}

func Run(c *catalog.Catalog, opts Options) error {
	hosts := target.HostsForSelector(c, opts.Selector)
	if len(hosts) == 0 {
		return fmt.Errorf("no hosts match %q", opts.Selector)
	}
	for _, h := range hosts {
		sshName := target.SSHName(h)
		remoteArgs := []string{}
		if opts.RemotePath != "" {
			remoteArgs = append(remoteArgs, "cd", shellQuote(opts.RemotePath), "&&")
		}
		remoteArgs = append(remoteArgs, "agentctl", opts.Subcommand)
		remoteArgs = append(remoteArgs, opts.Args...)
		cmd := exec.Command("ssh", sshName, strings.Join(remoteArgs, " "))
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s: %w", sshName, err)
		}
	}
	return nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
