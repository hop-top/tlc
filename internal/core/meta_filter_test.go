package core

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateMetaFilterKey(t *testing.T) {
	for _, k := range []string{"owner", "a", "A1", "snake_case", "kebab-case", "_x", "k9", strings.Repeat("a", 64)} {
		if err := ValidateMetaFilterKey(k); err != nil {
			t.Errorf("ValidateMetaFilterKey(%q) = %v, want nil", k, err)
		}
	}
	for _, k := range []string{
		"", " ", "a b", "a.b", "a'b", `a"b`, "a;b", "a)", "-x", "a\nb", "$", "a]",
		"x') OR 1=1 --", `x" OR "1"="1`, "a/*", "é", strings.Repeat("a", 65),
	} {
		err := ValidateMetaFilterKey(k)
		if !errors.Is(err, ErrInvalidMetaFilterKey) {
			t.Errorf("ValidateMetaFilterKey(%q) = %v, want ErrInvalidMetaFilterKey", k, err)
		}
	}
}

func TestValidateMetaFilters(t *testing.T) {
	if err := ValidateMetaFilters(nil); err != nil {
		t.Fatalf("nil: %v", err)
	}
	if err := ValidateMetaFilters(map[string]string{"owner": "ana", "env": "a=b", "empty": ""}); err != nil {
		t.Fatalf("valid filters rejected: %v", err)
	}
	for _, bad := range []string{"a.b", "x') OR 1=1 --", ""} {
		err := ValidateMetaFilters(map[string]string{"ok": "1", bad: "1"})
		if !errors.Is(err, ErrInvalidMetaFilterKey) {
			t.Errorf("key %q: err = %v, want ErrInvalidMetaFilterKey", bad, err)
		}
	}
}
