// SPDX-License-Identifier: MIT

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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
