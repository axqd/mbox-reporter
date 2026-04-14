package gmail

import (
	"errors"
	"testing"
)

func TestIsInvalidGrantError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"unrelated error", errors.New("some other error"), false},
		{"contains invalid_grant", errors.New(`auth: cannot fetch token: 400 Response: {"error": "invalid_grant"}`), true},
		{"plain invalid_grant", errors.New("invalid_grant"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isInvalidGrantError(tt.err); got != tt.want {
				t.Errorf("isInvalidGrantError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
