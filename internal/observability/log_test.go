// SPDX-License-Identifier: MIT

package observability

import (
	"bytes"
	"log"
	"log/slog"
	"strings"
	"testing"
)

func TestStructuredLoggerRedactsSecrets(t *testing.T) {
	var out bytes.Buffer
	Configure(&out, slog.LevelInfo)
	Event(slog.LevelInfo, "login_failed", "failed", "client", "192.0.2.1", "password", "secret", "access_token", "bearer", "safe", "visible")
	text := out.String()
	for _, secret := range []string{"secret", "bearer"} {
		if strings.Contains(text, secret) {
			t.Fatalf("log leaked %q: %s", secret, text)
		}
	}
	for _, want := range []string{"event=login_failed", "safe=visible", "[REDACTED]"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
}

func TestLegacyLoggerGetsStableEvent(t *testing.T) {
	var out bytes.Buffer
	Configure(&out, slog.LevelInfo)
	log.Print("old message")
	if text := out.String(); !strings.Contains(text, "event=legacy_log") {
		t.Fatalf("legacy log not structured: %s", text)
	}
}
