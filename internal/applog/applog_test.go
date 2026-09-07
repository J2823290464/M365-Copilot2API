package applog

import (
	"bytes"
	"strings"
	"testing"
)

func TestUnifiedFormat(t *testing.T) {
	var output bytes.Buffer
	SetOutput(&output)
	t.Cleanup(func() { SetOutput(nil) })

	Info("web", "request_completed", "request_id", "req-1", "status", 200)
	line := output.String()
	for _, expected := range []string{"level=info", "event=request_completed", "module=web", "request_id=req-1", "status=200"} {
		if !strings.Contains(line, expected) {
			t.Fatalf("log line %q does not contain %q", line, expected)
		}
	}
}

func TestNilValuesAreReadable(t *testing.T) {
	if got := StringValue(nil); got != "<nil>" {
		t.Fatalf("StringValue(nil) = %q", got)
	}
	if got := ErrorValue(nil); got != "<nil>" {
		t.Fatalf("ErrorValue(nil) = %v", got)
	}
}
