// SPDX-License-Identifier: MIT

package middleware

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func FuzzSessionMetadataDecode(f *testing.F) {
	f.Add([]byte(`[{"id":"bad","digest":"00","expires":1}]`))
	f.Add([]byte(`null`))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 256<<10 {
			t.Skip()
		}
		var entries []persistedSession
		if json.Unmarshal(data, &entries) != nil || len(entries) > maxSessions {
			return
		}
		for _, entry := range entries {
			_, _ = hex.DecodeString(entry.Digest)
			_ = validSessionID(entry.ID)
		}
	})
}

func FuzzForwardedAndOriginHeaders(f *testing.F) {
	for _, seed := range [][3]string{{"https://example.test", "127.0.0.1:1", "for=192.0.2.1;proto=https"}, {"null", "[::1]:2", "for=\"[2001:db8::1]\""}, {"https://evil.test/%0d%0aX:x", "bad", ";;;;"}} {
		f.Add(seed[0], seed[1], seed[2])
	}
	f.Fuzz(func(t *testing.T, origin, remote, forwarded string) {
		if len(origin)+len(remote)+len(forwarded) > 16<<10 {
			t.Skip()
		}
		r := httptest.NewRequest("POST", "http://example.test/api/link", nil)
		r.RemoteAddr = remote
		r.Header.Set("Origin", origin)
		r.Header.Set("Forwarded", forwarded)
		r.Header.Set("X-Forwarded-For", forwarded)
		w := httptest.NewRecorder()
		WithSecurity(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })(w, r)
	})
}
