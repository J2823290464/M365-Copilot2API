package web

import (
	"os"
	"testing"
	"time"
)

func TestApplyTimezoneEnv(t *testing.T) {
	previousLocation := time.Local
	previousTimezone, hadTimezone := os.LookupEnv("M365_TIMEZONE")
	t.Cleanup(func() {
		time.Local = previousLocation
		if hadTimezone {
			_ = os.Setenv("M365_TIMEZONE", previousTimezone)
		} else {
			_ = os.Unsetenv("M365_TIMEZONE")
		}
	})

	if err := os.Setenv("M365_TIMEZONE", "Asia/Shanghai"); err != nil {
		t.Fatal(err)
	}
	if err := ApplyTimezoneEnv(); err != nil {
		t.Fatal(err)
	}
	if got := time.Now().Location().String(); got != "Asia/Shanghai" {
		t.Fatalf("time.Local = %q, want Asia/Shanghai", got)
	}
}

func TestApplyTimezoneEnvRejectsUnknownTimezone(t *testing.T) {
	previousTimezone, hadTimezone := os.LookupEnv("M365_TIMEZONE")
	t.Cleanup(func() {
		if hadTimezone {
			_ = os.Setenv("M365_TIMEZONE", previousTimezone)
		} else {
			_ = os.Unsetenv("M365_TIMEZONE")
		}
	})

	if err := os.Setenv("M365_TIMEZONE", "Invalid/Timezone"); err != nil {
		t.Fatal(err)
	}
	if err := ApplyTimezoneEnv(); err == nil {
		t.Fatal("ApplyTimezoneEnv() succeeded for an unknown timezone")
	}
}
