package metric

import (
	"testing"
)

const statusLabel = "status"

func TestCounter_Inc_Table(t *testing.T) {
	tests := []struct {
		name       string
		help       string
		labelName  string
		labelValue string
	}{
		{"test_success", "success counter", statusLabel, "success"},
		{"test_failure", "failure counter", statusLabel, "failure"},
		{"test_pending", "pending counter", statusLabel, "pending"},
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
	c1.Increment("a")
	c2.Increment("b")
}
