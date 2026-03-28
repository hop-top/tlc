package config

import (
	"fmt"
	"regexp"
)

// ValidationOperation identifies a task operation that validation applies to.
type ValidationOperation string

const (
	ValidationOpCreate ValidationOperation = "create"
	ValidationOpUpdate ValidationOperation = "update"
	ValidationOpDelete ValidationOperation = "delete"
)

// knownTaskFields lists the task field names accepted in validation rules.
var knownTaskFields = map[string]bool{
	"title":       true,
	"description": true,
	"status":      true,
	"assigned_to": true,
	"effort":      true,
	"priority":    true,
	"tags":        true,
	"reference":   true,
}

// ValidationRule defines a regex pattern a field value must satisfy.
type ValidationRule struct {
	Field   string `yaml:"field"`
	Pattern string `yaml:"pattern"`
	Message string `yaml:"message,omitempty"` // optional human-readable error
}

// ValidationOpConfig holds required fields and rules for a single operation.
type ValidationOpConfig struct {
	RequiredFields []string         `yaml:"required_fields,omitempty"`
	Rules          []ValidationRule `yaml:"rules,omitempty"`
}

// ValidationConfig is the top-level config section for custom task validation.
type ValidationConfig struct {
	Create ValidationOpConfig `yaml:"create,omitempty"`
	Update ValidationOpConfig `yaml:"update,omitempty"`
	Delete ValidationOpConfig `yaml:"delete,omitempty"`
}

// Validate checks ValidationConfig for structural correctness.
// Compiles all regex patterns and ensures field names are known.
func (v *ValidationConfig) Validate() error {
	for _, op := range []struct {
		name string
		cfg  ValidationOpConfig
	}{
		{"create", v.Create},
		{"update", v.Update},
		{"delete", v.Delete},
	} {
		for _, f := range op.cfg.RequiredFields {
			if !knownTaskFields[f] {
				return fmt.Errorf("validation.%s.required_fields: unknown field %q", op.name, f)
			}
		}
		for i, r := range op.cfg.Rules {
			if !knownTaskFields[r.Field] {
				return fmt.Errorf("validation.%s.rules[%d]: unknown field %q", op.name, i, r.Field)
			}
			if r.Pattern == "" {
				return fmt.Errorf("validation.%s.rules[%d]: pattern must not be empty", op.name, i)
			}
			if _, err := regexp.Compile(r.Pattern); err != nil {
				return fmt.Errorf(
					"validation.%s.rules[%d]: invalid pattern %q: %w", op.name, i, r.Pattern, err,
				)
			}
		}
	}
	return nil
}

// TaskFields is a flat view of a task used during validation.
// Callers populate only the fields relevant to the operation.
type TaskFields struct {
	Title       string
	Description string
	Status      string
	AssignedTo  string
	Effort      string
	Priority    string
	Tags        []string
	Reference   string
}

func (tf TaskFields) get(field string) string {
	switch field {
	case "title":
		return tf.Title
	case "description":
		return tf.Description
	case "status":
		return tf.Status
	case "assigned_to":
		return tf.AssignedTo
	case "effort":
		return tf.Effort
	case "priority":
		return tf.Priority
	case "tags":
		// Represent tags as comma-joined string for pattern matching.
		result := ""
		for i, t := range tf.Tags {
			if i > 0 {
				result += ","
			}
			result += t
		}
		return result
	case "reference":
		return tf.Reference
	default:
		return ""
	}
}

// ValidateTaskOp runs all configured validations for the given operation
// against the supplied task fields. Returns the first error encountered.
func (v *ValidationConfig) ValidateTaskOp(op ValidationOperation, fields TaskFields) error {
	var cfg ValidationOpConfig
	switch op {
	case ValidationOpCreate:
		cfg = v.Create
	case ValidationOpUpdate:
		cfg = v.Update
	case ValidationOpDelete:
		cfg = v.Delete
	default:
		return fmt.Errorf("unknown operation: %s", op)
	}

	// Required-fields check.
	for _, f := range cfg.RequiredFields {
		val := fields.get(f)
		if val == "" {
			return fmt.Errorf("field %q is required for %s", f, op)
		}
	}

	// Pattern rules.
	for _, r := range cfg.Rules {
		val := fields.get(r.Field)
		matched, err := regexp.MatchString(r.Pattern, val)
		if err != nil {
			// Pattern was already validated during config load; this should not happen.
			return fmt.Errorf("rule pattern error for field %q: %w", r.Field, err)
		}
		if !matched {
			msg := r.Message
			if msg == "" {
				msg = fmt.Sprintf("field %q value %q does not match pattern %q", r.Field, val, r.Pattern)
			}
			return fmt.Errorf("%s validation failed for %s: %s", op, r.Field, msg)
		}
	}

	return nil
}
