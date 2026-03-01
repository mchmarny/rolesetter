package metric

import (
	"testing"
)

func TestCounter_Inc_Table(t *testing.T) {
	tests := []struct {
		name       string
		help       string
		labelName  string
		labelValue string
	}{
		{"test_success", "success counter", "status", "success"},
		{"test_failure", "failure counter", "status", "failure"},
		{"test_pending", "pending counter", "status", "pending"},
	}
	for _, tt := range tests {
		c := NewCounter(tt.name, tt.help, tt.labelName)
		t.Run(tt.name, func(t *testing.T) {
			if c == nil {
				t.Fatalf("NewCounter(%q, %q, %q) returned nil", tt.name, tt.help, tt.labelName)
			}
			c.Increment(tt.labelValue)
		})
	}
}

func TestCounter_SafeReRegistration(t *testing.T) {
	name := "test_safe_rereg"
	c1 := NewCounter(name, "first", "label")
	c2 := NewCounter(name, "first", "label")
	if c1 == nil || c2 == nil {
		t.Fatal("NewCounter returned nil on re-registration")
	}
	// Both should work without panic
	c1.Increment("a")
	c2.Increment("b")
}
