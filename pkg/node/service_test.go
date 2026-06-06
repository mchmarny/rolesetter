package node

import (
	"testing"

	"github.com/mchmarny/rolesetter/pkg/logger"
)

func TestOptionsFromEnv_Success(t *testing.T) {
	t.Setenv(envRoleLabel, "nodeGroup")
	t.Setenv(envServerPort, "9090")
	t.Setenv(envRoleLabelReplace, "yes")
	t.Setenv(envNamespace, "system")

	opts, err := optionsFromEnv(logger.GetTestLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	inf := &Informer{}
	for _, o := range opts {
		o(inf)
	}
	if inf.label != "nodeGroup" {
		t.Errorf("label = %s", inf.label)
	}
	if inf.port != 9090 {
		t.Errorf("port = %d", inf.port)
	}
	if !inf.replace {
		t.Error("replace expected true")
	}
	if inf.namespace != "system" {
		t.Errorf("namespace = %s", inf.namespace)
	}
}

func TestOptionsFromEnv_Defaults(t *testing.T) {
	t.Setenv(envRoleLabel, "nodeGroup")
	t.Setenv(envServerPort, "")
	t.Setenv(envRoleLabelReplace, "")
	t.Setenv(envNamespace, "")

	opts, err := optionsFromEnv(logger.GetTestLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	inf := &Informer{}
	for _, o := range opts {
		o(inf)
	}
	if inf.port != 8080 {
		t.Errorf("expected default port 8080, got %d", inf.port)
	}
	if inf.replace {
		t.Error("replace should default false")
	}
	if inf.namespace != "" {
		t.Errorf("namespace should be empty, got %s", inf.namespace)
	}
}

func TestOptionsFromEnv_Errors(t *testing.T) {
	tests := []struct {
		name string
		set  map[string]string
	}{
		{"missing label", map[string]string{envRoleLabel: ""}},
		{"bad port", map[string]string{envRoleLabel: "x", envServerPort: "abc"}},
		{"zero port", map[string]string{envRoleLabel: "x", envServerPort: "0"}},
		{"bad bool", map[string]string{envRoleLabel: "x", envRoleLabelReplace: "maybe"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.set {
				t.Setenv(k, v)
			}
			if _, err := optionsFromEnv(logger.GetTestLogger()); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestParseBoolish(t *testing.T) {
	tests := []struct {
		in      string
		want    bool
		wantErr bool
	}{
		{"", false, false},
		{"true", true, false},
		{"TRUE", true, false},
		{"1", true, false},
		{"yes", true, false},
		{"y", true, false},
		{"false", false, false},
		{"0", false, false},
		{"no", false, false},
		{"n", false, false},
		{"maybe", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseBoolish(tt.in)
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
