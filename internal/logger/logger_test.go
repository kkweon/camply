package logger

import (
	"bytes"
	"strings"
	"testing"
)

// Results are printed at INFO on stdout. A warning that joined them there would
// corrupt the output of anyone piping stdout, so the split is worth locking down.
func TestLevelsRouteToCorrectStream(t *testing.T) {
	tests := []struct {
		name     string
		log      func(string, ...interface{})
		wantOn   string
		wantTag  string
		otherOne string
	}{
		{"info to stdout", Info, "stdout", "INFO", "stderr"},
		{"camply to stdout", Camply, "stdout", "CAMPLY", "stderr"},
		{"warn to stderr", Warn, "stderr", "WARNING", "stdout"},
		{"error to stderr", Error, "stderr", "ERROR", "stdout"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			SetOutput(&out, &errOut)
			t.Cleanup(ResetOutput)
			Setup()

			tt.log("hello %s", "world")

			got, other := out.String(), errOut.String()
			if tt.wantOn == "stderr" {
				got, other = errOut.String(), out.String()
			}
			if !strings.Contains(got, "hello world") {
				t.Errorf("%s: message missing from %s, got %q", tt.name, tt.wantOn, got)
			}
			if !strings.Contains(got, tt.wantTag) {
				t.Errorf("%s: level tag %q missing, got %q", tt.name, tt.wantTag, got)
			}
			if other != "" {
				t.Errorf("%s: %s should be empty, got %q", tt.name, tt.otherOne, other)
			}
		})
	}
}

func TestDebugSuppressedUnlessEnabled(t *testing.T) {
	var out, errOut bytes.Buffer
	SetOutput(&out, &errOut)
	t.Cleanup(func() { ResetOutput(); SetDebug(false) })

	SetDebug(false)
	Debug("quiet please")
	if out.Len() != 0 || errOut.Len() != 0 {
		t.Fatalf("debug leaked while disabled: stdout=%q stderr=%q", out.String(), errOut.String())
	}

	SetDebug(true)
	Debug("now visible")
	if !strings.Contains(out.String(), "now visible") {
		t.Fatalf("debug missing while enabled, got %q", out.String())
	}
}
