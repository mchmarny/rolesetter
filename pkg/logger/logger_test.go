package logger

import (
	"testing"

	"go.uber.org/zap/zapcore"
)

func TestGetLogger_NotNil(t *testing.T) {
	l := GetLogger()
	if l == nil {
		t.Error("GetLogger returned nil")
	}
}

func TestGetDebugLogger_NotNil(t *testing.T) {
	l := GetDebugLogger()
	if l == nil {
		t.Error("GetDebugLogger returned nil")
	}
}

func TestGetTestLogger_NotNil(t *testing.T) {
	l := GetTestLogger()
	if l == nil {
		t.Error("GetTestLogger returned nil")
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		in    string
		level zapcore.Level
		known bool
	}{
		{"debug", zapcore.DebugLevel, true},
		{"info", zapcore.InfoLevel, true},
		{"warn", zapcore.WarnLevel, true},
		{"error", zapcore.ErrorLevel, true},
		{"", zapcore.InfoLevel, true},
		{"verbose", zapcore.InfoLevel, false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, ok := parseLevel(tt.in)
			if got != tt.level || ok != tt.known {
				t.Errorf("parseLevel(%q) = (%v, %v), want (%v, %v)", tt.in, got, ok, tt.level, tt.known)
			}
		})
	}
}

func TestGetLogger_DefaultsOnInvalidLevel(t *testing.T) {
	t.Setenv("LOG_LEVEL", "verbose")
	l := GetLogger()
	if l == nil {
		t.Fatal("GetLogger returned nil")
	}
	if !l.Core().Enabled(zapcore.InfoLevel) {
		t.Error("expected info level enabled")
	}
}
