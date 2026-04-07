package core

import (
	"strings"
	"testing"
)

func TestSubstituteFlowInputs(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		inputs map[string]any
		want   string
	}{
		{
			name:   "known key substituted",
			in:     "hello {{name}}",
			inputs: map[string]any{"name": "world"},
			want:   "hello world",
		},
		{
			name:   "whitespace variant",
			in:     "{{ name }}",
			inputs: map[string]any{"name": "x"},
			want:   "x",
		},
		{
			name:   "unknown placeholder left untouched",
			in:     "{{foo}}",
			inputs: map[string]any{},
			want:   "{{foo}}",
		},
		{
			name:   "known and unknown in same string",
			in:     "{{name}} vs {{foo}}",
			inputs: map[string]any{"name": "real"},
			want:   "real vs {{foo}}",
		},
		{
			name:   "multiple occurrences of same key",
			in:     "{{x}} and {{x}} and {{x}}",
			inputs: map[string]any{"x": "y"},
			want:   "y and y and y",
		},
		{
			name:   "non-input syntax untouched",
			in:     "${VAR}",
			inputs: map[string]any{"VAR": "x"},
			want:   "${VAR}",
		},
		{
			name:   "empty string",
			in:     "",
			inputs: map[string]any{"x": "y"},
			want:   "",
		},
		{
			name:   "empty inputs map",
			in:     "hello {{name}}",
			inputs: map[string]any{},
			want:   "hello {{name}}",
		},
		{
			name:   "nil inputs map",
			in:     "hello {{name}}",
			inputs: nil,
			want:   "hello {{name}}",
		},
		{
			name:   "integer value renders via %v",
			in:     "count: {{count}}",
			inputs: map[string]any{"count": 42},
			want:   "count: 42",
		},
		{
			name:   "mixed whitespace variants",
			in:     "{{a}}/{{ b }}/{{  c  }}",
			inputs: map[string]any{"a": "1", "b": "2", "c": "3"},
			want:   "1/2/3",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SubstituteFlowInputs(tc.in, tc.inputs)
			if got != tc.want {
				t.Errorf("SubstituteFlowInputs(%q, %v)\n  got:  %q\n  want: %q",
					tc.in, tc.inputs, got, tc.want)
			}
		})
	}
}

func TestParseFlowVarFlags(t *testing.T) {
	tests := []struct {
		name    string
		in      []string
		want    map[string]string
		wantErr string
	}{
		{
			name: "valid single",
			in:   []string{"k=v"},
			want: map[string]string{"k": "v"},
		},
		{
			name: "valid multiple",
			in:   []string{"a=1", "b=2"},
			want: map[string]string{"a": "1", "b": "2"},
		},
		{
			name: "value with equals",
			in:   []string{"url=https://example.com?k=v"},
			want: map[string]string{"url": "https://example.com?k=v"},
		},
		{
			name:    "missing equals",
			in:      []string{"noequals"},
			wantErr: "invalid --var",
		},
		{
			name:    "empty key",
			in:      []string{"=value"},
			wantErr: "invalid --var",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseFlowVarFlags(tc.in)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %d entries, want %d: %v", len(got), len(tc.want), got)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("key %q: got %q, want %q", k, got[k], v)
				}
			}
		})
	}
}
