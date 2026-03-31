package conv

import (
	"testing"
)

func TestAsString(t *testing.T) {
	tests := []struct {
		input any
		want  string
	}{
		{nil, ""},
		{"hello", "hello"},
		{42, "42"},
		{3.14, "3.14"},
		{true, "true"},
	}
	for _, tt := range tests {
		got := AsString(tt.input)
		if got != tt.want {
			t.Errorf("AsString(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestAsStringSlice(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if got := AsStringSlice(nil); got != nil {
			t.Errorf("AsStringSlice(nil) = %v, want nil", got)
		}
	})

	t.Run("string slice", func(t *testing.T) {
		input := []string{"a", "b"}
		got := AsStringSlice(input)
		if len(got) != 2 || got[0] != "a" || got[1] != "b" {
			t.Errorf("AsStringSlice([]string) = %v, want [a b]", got)
		}
	})

	t.Run("any slice", func(t *testing.T) {
		input := []any{"x", 42, nil}
		got := AsStringSlice(input)
		if len(got) != 3 || got[0] != "x" || got[1] != "42" || got[2] != "" {
			t.Errorf("AsStringSlice([]any) = %v, want [x 42 ]", got)
		}
	})

	t.Run("unsupported", func(t *testing.T) {
		if got := AsStringSlice("not a slice"); got != nil {
			t.Errorf("AsStringSlice(string) = %v, want nil", got)
		}
	})
}

func TestAsBool(t *testing.T) {
	tests := []struct {
		input any
		want  bool
	}{
		{nil, false},
		{true, true},
		{false, false},
		{"true", true},
		{"TRUE", true},
		{" True ", true},
		{"false", false},
		{"", false},
		{42, false},
	}
	for _, tt := range tests {
		got := AsBool(tt.input)
		if got != tt.want {
			t.Errorf("AsBool(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
