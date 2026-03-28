package config

import (
	"strings"
	"testing"
)

func TestValidationConfig_Validate_Valid(t *testing.T) {
	cfg := ValidationConfig{
		Create: ValidationOpConfig{
			RequiredFields: []string{"title", "description"},
			Rules: []ValidationRule{
				{Field: "title", Pattern: `^.{3,}$`, Message: "title must be at least 3 chars"},
			},
		},
		Update: ValidationOpConfig{
			RequiredFields: []string{"title"},
		},
		Delete: ValidationOpConfig{
			Rules: []ValidationRule{
				{Field: "status", Pattern: `^(DONE|SKIPPED)$`, Message: "only done tasks may be deleted"},
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidationConfig_Validate_UnknownField(t *testing.T) {
	cfg := ValidationConfig{
		Create: ValidationOpConfig{
			RequiredFields: []string{"nonexistent"},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for unknown required field")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention field name, got: %v", err)
	}
}

func TestValidationConfig_Validate_BadPattern(t *testing.T) {
	cfg := ValidationConfig{
		Update: ValidationOpConfig{
			Rules: []ValidationRule{
				{Field: "title", Pattern: `[invalid`},
			},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid pattern")
	}
	if !strings.Contains(err.Error(), "invalid pattern") {
		t.Errorf("error should mention invalid pattern, got: %v", err)
	}
}

func TestValidationConfig_Validate_EmptyPattern(t *testing.T) {
	cfg := ValidationConfig{
		Delete: ValidationOpConfig{
			Rules: []ValidationRule{
				{Field: "title", Pattern: ""},
			},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty pattern")
	}
}

func TestValidateTaskOp_RequiredField_Missing(t *testing.T) {
	cfg := ValidationConfig{
		Create: ValidationOpConfig{
			RequiredFields: []string{"description"},
		},
	}
	err := cfg.ValidateTaskOp(ValidationOpCreate, TaskFields{
		Title:       "Some title",
		Description: "",
	})
	if err == nil {
		t.Fatal("expected error for missing required field")
	}
	if !strings.Contains(err.Error(), "description") {
		t.Errorf("error should mention field name, got: %v", err)
	}
}

func TestValidateTaskOp_RequiredField_Present(t *testing.T) {
	cfg := ValidationConfig{
		Create: ValidationOpConfig{
			RequiredFields: []string{"description"},
		},
	}
	err := cfg.ValidateTaskOp(ValidationOpCreate, TaskFields{
		Title:       "Some title",
		Description: "Some description",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidateTaskOp_Rule_Passes(t *testing.T) {
	cfg := ValidationConfig{
		Create: ValidationOpConfig{
			Rules: []ValidationRule{
				{Field: "title", Pattern: `^.{5,}$`, Message: "title too short"},
			},
		},
	}
	err := cfg.ValidateTaskOp(ValidationOpCreate, TaskFields{Title: "Hello World"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidateTaskOp_Rule_Fails(t *testing.T) {
	cfg := ValidationConfig{
		Create: ValidationOpConfig{
			Rules: []ValidationRule{
				{Field: "title", Pattern: `^.{10,}$`, Message: "title too short"},
			},
		},
	}
	err := cfg.ValidateTaskOp(ValidationOpCreate, TaskFields{Title: "Hi"})
	if err == nil {
		t.Fatal("expected error for pattern mismatch")
	}
	if !strings.Contains(err.Error(), "title too short") {
		t.Errorf("error should use custom message, got: %v", err)
	}
}

func TestValidateTaskOp_Rule_DefaultMessage(t *testing.T) {
	cfg := ValidationConfig{
		Update: ValidationOpConfig{
			Rules: []ValidationRule{
				{Field: "priority", Pattern: `^P[0-3]$`},
			},
		},
	}
	err := cfg.ValidateTaskOp(ValidationOpUpdate, TaskFields{Priority: "invalid"})
	if err == nil {
		t.Fatal("expected error for pattern mismatch")
	}
	if !strings.Contains(err.Error(), "priority") {
		t.Errorf("error should mention field, got: %v", err)
	}
}

func TestValidateTaskOp_Delete_StatusRule(t *testing.T) {
	cfg := ValidationConfig{
		Delete: ValidationOpConfig{
			Rules: []ValidationRule{
				{Field: "status", Pattern: `^(DONE|SKIPPED)$`, Message: "only terminal tasks may be deleted"},
			},
		},
	}

	if err := cfg.ValidateTaskOp(ValidationOpDelete, TaskFields{Status: "DONE"}); err != nil {
		t.Errorf("expected no error for DONE status, got: %v", err)
	}

	err := cfg.ValidateTaskOp(ValidationOpDelete, TaskFields{Status: "TODO"})
	if err == nil {
		t.Fatal("expected error for non-terminal status")
	}
	if !strings.Contains(err.Error(), "only terminal tasks may be deleted") {
		t.Errorf("expected custom message, got: %v", err)
	}
}

func TestValidateTaskOp_TagsField(t *testing.T) {
	cfg := ValidationConfig{
		Create: ValidationOpConfig{
			RequiredFields: []string{"tags"},
		},
	}

	// Empty tags should fail.
	if err := cfg.ValidateTaskOp(ValidationOpCreate, TaskFields{Tags: []string{}}); err == nil {
		t.Fatal("expected error for empty tags")
	}

	// Non-empty tags should pass.
	if err := cfg.ValidateTaskOp(ValidationOpCreate, TaskFields{Tags: []string{"feat"}}); err != nil {
		t.Fatalf("expected no error with tags, got: %v", err)
	}
}

func TestValidateTaskOp_NoConfig_Noop(t *testing.T) {
	cfg := ValidationConfig{}
	fields := TaskFields{Title: "", Description: "", Status: "TODO"}
	if err := cfg.ValidateTaskOp(ValidationOpCreate, fields); err != nil {
		t.Fatalf("empty config should be no-op, got: %v", err)
	}
}
