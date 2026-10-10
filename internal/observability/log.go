// SPDX-License-Identifier: MIT

package observability

import (
	"context"
	"io"
	"log"
	"log/slog"
	"strings"
)

var secretKeys = map[string]bool{"password": true, "pass": true, "token": true, "cookie": true, "authorization": true, "proxy_password": true, "query": true, "url": true}

type redactingHandler struct{ next slog.Handler }

func (h redactingHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.next.Enabled(ctx, l)
}
func (h redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return redactingHandler{h.next.WithAttrs(redact(attrs))}
}
func (h redactingHandler) WithGroup(name string) slog.Handler {
	return redactingHandler{h.next.WithGroup(name)}
}
func (h redactingHandler) Handle(ctx context.Context, r slog.Record) error {
	clean := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool { clean.AddAttrs(redactOne(a)); return true })
	return h.next.Handle(ctx, clean)
}
func redact(attrs []slog.Attr) []slog.Attr {
	out := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		out[i] = redactOne(a)
	}
	return out
}
func redactOne(a slog.Attr) slog.Attr {
	key := strings.ToLower(a.Key)
	if secretKeys[key] || strings.Contains(key, "password") || strings.Contains(key, "token") || strings.Contains(key, "cookie") {
		return slog.String(a.Key, "[REDACTED]")
	}
	return a
}

type bridgeWriter struct{ logger *slog.Logger }

func (w bridgeWriter) Write(p []byte) (int, error) {
	w.logger.Info(strings.TrimSpace(string(p)), "event", "legacy_log")
	return len(p), nil
}

func Configure(w io.Writer, level slog.Level) {
	handler := redactingHandler{slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})}
	logger := slog.New(handler)
	slog.SetDefault(logger)
	log.SetFlags(0)
	log.SetOutput(bridgeWriter{logger})
}

func Event(level slog.Level, event, msg string, attrs ...any) {
	slog.Log(context.Background(), level, msg, append([]any{"event", event}, attrs...)...)
}
