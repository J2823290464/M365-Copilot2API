package web

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// ApplyTimezoneEnv makes time.Now and the standard logger use the configured
// application timezone instead of relying on the host/container default.
func ApplyTimezoneEnv() error {
	zone := strings.TrimSpace(os.Getenv("M365_TIMEZONE"))
	if zone == "" {
		zone = strings.TrimSpace(os.Getenv("TZ"))
	}
	if zone == "" {
		return nil
	}

	location, err := time.LoadLocation(zone)
	if err != nil {
		return fmt.Errorf("load timezone %q: %w", zone, err)
	}
	time.Local = location
	return nil
}
