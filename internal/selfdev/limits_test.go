package selfdev

import (
	"testing"
	"time"
)

func TestM0TimeoutBudgets(t *testing.T) {
	for _, tt := range []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{"verification", VerificationTimeout, 15 * time.Minute},
		{"ready", ReadyTimeout, 30 * time.Second},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("timeout = %s, want %s; update the M0 measurement record when changing limits", tt.got, tt.want)
			}
		})
	}
}
