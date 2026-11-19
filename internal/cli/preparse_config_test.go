package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// A flag value that merely looks like a -c cluster (leading "-", a 'c'
// inside) must never be lifted as a config token: it is the value of the
// flag that precedes it.
func TestPreParseConfigTokens_FlagValuesAreNotConfig(t *testing.T) {
	long := "- " + strings.Repeat("see /a/b/c/d ", 100)
	cases := map[string][]string{
		"update -d dash text":      {"tlc", "task", "update", "T-1", "-d", "- cat /a/b/c.go"},
		"update -d long dash text": {"tlc", "task", "update", "T-1", "-d", long},
		"update --description":     {"tlc", "task", "update", "T-1", "--description", "-c /a/b"},
		"create -d dash text":      {"tlc", "task", "create", "t", "-d", "- cat x"},
		"create --description=":    {"tlc", "task", "create", "t", "--description=- cat x"},
		"update --title":           {"tlc", "task", "update", "T-1", "--title", "-x cat /y"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Empty(t, preParseConfigTokens(args))
		})
	}
}

// Real -c tokens keep working in every spelling, next to flags whose
// values start with "-".
func TestPreParseConfigTokens_StillLiftsConfig(t *testing.T) {
	cases := map[string][]string{
		"-c v":      {"tlc", "task", "list", "-c", "x.yaml"},
		"-cv":       {"tlc", "task", "list", "-cx.yaml"},
		"-c=v":      {"tlc", "task", "list", "-c=x.yaml"},
		"--config":  {"tlc", "task", "list", "--config", "x.yaml"},
		"--config=": {"tlc", "task", "list", "--config=x.yaml"},
		"after -d":  {"tlc", "task", "update", "T-1", "-d", "- cat", "-c", "x.yaml"},
		"help":      {"tlc", "--help", "-c", "x.yaml"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, []string{"x.yaml"}, preParseConfigTokens(args))
		})
	}
}
