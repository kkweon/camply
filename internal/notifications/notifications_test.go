package notifications

import (
	"encoding/base64"
	"testing"

	"github.com/kkweon/camply/internal/config"
)

// TestNewPushover_TokenOptional guards a divergence from the Python implementation:
// Go required both PUSHOVER_PUSH_TOKEN and PUSHOVER_PUSH_USER, so a config with only
// a user key -- the documented setup -- failed outright. The token identifies the
// sending application, not the recipient, so camply falls back to its own app.
func TestNewPushover_TokenOptional(t *testing.T) {
	wantDefault, err := base64.StdEncoding.DecodeString(pushoverDefaultAPIToken)
	if err != nil {
		t.Fatalf("default token is not valid base64: %v", err)
	}

	tests := []struct {
		name      string
		cfg       *config.AppConfig
		wantErr   bool
		wantToken string
	}{
		{
			name:      "user only falls back to camply's app token",
			cfg:       &config.AppConfig{PushoverPushUser: "user-key"},
			wantToken: string(wantDefault),
		},
		{
			name:      "explicit token is preserved",
			cfg:       &config.AppConfig{PushoverPushUser: "user-key", PushoverPushToken: "my-app-token"},
			wantToken: "my-app-token",
		},
		{
			name:    "missing user is still an error",
			cfg:     &config.AppConfig{PushoverPushToken: "my-app-token"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewPushover(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			p, ok := got.(*pushoverNotifier)
			if !ok {
				t.Fatalf("expected *pushoverNotifier, got %T", got)
			}
			if p.token != tt.wantToken {
				t.Errorf("token = %q, want %q", p.token, tt.wantToken)
			}
		})
	}
}
