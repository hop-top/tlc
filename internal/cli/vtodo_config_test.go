package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/vtodo"
)

// exportVtodoWithConfig writes the supplied `output.vtodo` YAML (indented
// two spaces, to nest under `output:`) into the test config file, seeds
// one task, and runs `task list`, returning the emitted calendar.
//
// The config goes through the file rather than viper.Set because
// cmd.Execute() runs initConfig, which rereads the config file and would
// discard a value set only in memory. Driving the real file is also the
// closer test: it is how a user actually sets these keys.
func exportVtodoWithConfig(t *testing.T, vtodoYAML string) string {
	t.Helper()

	// output.format goes in the same file rather than on the command
	// line: TaskCmd is a package-level cobra command whose --format flag
	// value survives between Execute() calls, so a flag set by one test
	// leaks into the next. The config file is reread per run.
	outputYAML := "output:\n  format: " + formatVtodo + "\n" + vtodoYAML
	_, _ = resetTestEnvWithScheduling(t, outputYAML)
	ctx := context.Background()

	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC()
	if err := s.CreateTask(ctx, &core.Task{
		ID: "T-0001", Title: "Configured export",
		Status: core.StatusTodo, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task list in vtodo format failed: %v", err)
	}
	return buf.String()
}

// TestVtodoExportHonorsConfiguredProductID is the wiring test for
// `output.vtodo.product_id`. The key is documented in the vtodo sync
// spec; until it was read here, every export carried the package
// default no matter what the user configured.
func TestVtodoExportHonorsConfiguredProductID(t *testing.T) {
	withTestLock(func() {
		const custom = "-//MyOrg//Calendar//EN"
		out := exportVtodoWithConfig(t,
			"  vtodo:\n    product_id: \""+custom+"\"\n")

		if !strings.Contains(out, "PRODID:"+custom) {
			t.Errorf("expected configured PRODID %q, got:\n%s", custom, out)
		}
		if strings.Contains(out, vtodo.DefaultProductID) {
			t.Errorf("default PRODID leaked despite config, got:\n%s", out)
		}
	})
}

// TestVtodoExportHonorsConfiguredUIDDomain is the wiring test for
// `output.vtodo.uid_domain`.
func TestVtodoExportHonorsConfiguredUIDDomain(t *testing.T) {
	withTestLock(func() {
		const domain = "workspace.example.com"
		out := exportVtodoWithConfig(t,
			"  vtodo:\n    uid_domain: \""+domain+"\"\n")

		if !strings.Contains(out, "@"+domain) {
			t.Errorf("expected configured UID domain %q, got:\n%s", domain, out)
		}
		if strings.Contains(out, "@"+vtodo.DefaultUIDDomain) {
			t.Errorf("default UID domain leaked despite config, got:\n%s", out)
		}
	})
}

// TestVtodoExportHonorsBothKeys — the two keys are independent and both
// apply to the same export.
func TestVtodoExportHonorsBothKeys(t *testing.T) {
	withTestLock(func() {
		const (
			custom = "-//MyOrg//Calendar//EN"
			domain = "workspace.example.com"
		)
		out := exportVtodoWithConfig(t,
			"  vtodo:\n    product_id: \""+custom+"\"\n    uid_domain: \""+domain+"\"\n")

		if !strings.Contains(out, "PRODID:"+custom) {
			t.Errorf("expected configured PRODID %q, got:\n%s", custom, out)
		}
		if !strings.Contains(out, "@"+domain) {
			t.Errorf("expected configured UID domain %q, got:\n%s", domain, out)
		}
	})
}

// TestVtodoExportDefaultsWhenUnset pins the fallback: with neither key
// in the config the output is what it was before the keys were wired.
func TestVtodoExportDefaultsWhenUnset(t *testing.T) {
	withTestLock(func() {
		out := exportVtodoWithConfig(t, "")

		if !strings.Contains(out, "PRODID:"+vtodo.DefaultProductID) {
			t.Errorf("expected default PRODID, got:\n%s", out)
		}
		if !strings.Contains(out, "@"+vtodo.DefaultUIDDomain) {
			t.Errorf("expected default UID domain, got:\n%s", out)
		}
	})
}

// TestVtodoExportBlankConfigKeepsDefaults — a key present but empty must
// not produce a blank PRODID or a trailing-"@" UID. Both are malformed
// iCalendar.
func TestVtodoExportBlankConfigKeepsDefaults(t *testing.T) {
	withTestLock(func() {
		out := exportVtodoWithConfig(t,
			"  vtodo:\n    product_id: \"\"\n    uid_domain: \"\"\n")

		if !strings.Contains(out, "PRODID:"+vtodo.DefaultProductID) {
			t.Errorf("blank product_id should fall back to default, got:\n%s", out)
		}
		if !strings.Contains(out, "@"+vtodo.DefaultUIDDomain) {
			t.Errorf("blank uid_domain should fall back to default, got:\n%s", out)
		}
		if strings.Contains(out, "@\r\n") || strings.Contains(out, "PRODID:\r\n") {
			t.Errorf("blank config produced a malformed property, got:\n%s", out)
		}
	})
}
