package probe

import (
	"testing"

	"github.com/jnatkins/agentctl/internal/catalog"
)

func TestEnvProbe(t *testing.T) {
	t.Setenv("AGENTCTL_TEST_TOKEN", "set")
	result := runOne(catalog.CredentialProbe{ID: "token", Type: "env", Env: "AGENTCTL_TEST_TOKEN"}, false)
	if result.Status != "ok" {
		t.Fatalf("status = %q, evidence = %q", result.Status, result.Evidence)
	}
}
