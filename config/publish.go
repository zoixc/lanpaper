// SPDX-License-Identifier: MIT

package config

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"log"
	"net/url"
	"strings"
	"sync/atomic"
)

// publishKeySet is the parsed form of PUBLISH_KEYS: SHA-256 digests plus the
// fingerprints used in log lines. The raw keys are not kept here, so nothing on
// the request path can leak one into a log or an error message.
type publishKeySet struct {
	digests      [][]byte
	fingerprints []string
}

var publishKeyPtr atomic.Pointer[publishKeySet]

// corsSet is the parsed form of CORS_ORIGINS. Origins are stored canonically
// (lower-cased scheme://host[:port]) because a browser always sends them that
// way, which keeps matching a plain string compare.
type corsSet struct {
	wildcard bool
	origins  []string
}

var corsPtr atomic.Pointer[corsSet]

// maxCORSOrigins bounds the per-request linear scan over configured origins.
const maxCORSOrigins = 64

// RefreshDerived re-parses the list-valued settings (publish keys, CORS
// origins) into the lock-free snapshots used on the request path. Load calls it
// through validate(); tests call it after changing Current.
func RefreshDerived() {
	keys := &publishKeySet{}
	seen := make(map[string]bool, len(Current.PublishKeys))
	for _, raw := range Current.PublishKeys {
		if len(keys.digests) >= MaxPublishKeys {
			log.Printf("Warning: ignoring PUBLISH_KEYS entries beyond the %d key limit", MaxPublishKeys)
			break
		}
		key := strings.TrimSpace(raw)
		if key == "" {
			continue
		}
		if len(key) < MinPublishKeyLen {
			log.Printf("Warning: ignoring a PUBLISH_KEYS entry shorter than %d characters", MinPublishKeyLen)
			continue
		}
		sum := sha256.Sum256([]byte(key))
		full := hex.EncodeToString(sum[:])
		if seen[full] {
			log.Printf("Warning: ignoring a duplicate PUBLISH_KEYS entry (%s)", full[:KeyFingerprintLen])
			continue
		}
		seen[full] = true
		keys.digests = append(keys.digests, sum[:])
		keys.fingerprints = append(keys.fingerprints, full[:KeyFingerprintLen])
	}
	publishKeyPtr.Store(keys)

	cors := &corsSet{}
	for _, raw := range Current.CORSOrigins {
		if len(cors.origins) >= maxCORSOrigins {
			log.Printf("Warning: ignoring CORS_ORIGINS entries beyond the %d origin limit", maxCORSOrigins)
			break
		}
		origin, ok := parseCORSOrigin(raw)
		if !ok {
			log.Printf("Warning: ignoring invalid CORS_ORIGINS entry %q (expected * or scheme://host[:port])", raw)
			continue
		}
		if origin == "*" {
			cors.wildcard = true
			continue
		}
		cors.origins = append(cors.origins, origin)
	}
	corsPtr.Store(cors)
}

// KeyFingerprint returns the log-safe identifier of a key digest: the first
// KeyFingerprintLen hex characters of its SHA-256. It is not reversible, but it
// ties log lines to one configured key.
func KeyFingerprint(digest []byte) string {
	full := hex.EncodeToString(digest)
	if len(full) < KeyFingerprintLen {
		return full
	}
	return full[:KeyFingerprintLen]
}

// MatchPublishKey reports whether candidate matches a configured publish key
// and returns that key's fingerprint. Every configured key is compared in
// constant time, so neither the key length nor its position in the list is
// observable, and an unconfigured server rejects without hashing.
func MatchPublishKey(candidate string) (string, bool) {
	if candidate == "" {
		return "", false
	}
	set := publishKeyPtr.Load()
	if set == nil || len(set.digests) == 0 {
		return "", false
	}
	sum := sha256.Sum256([]byte(candidate))
	matched := -1
	for i, digest := range set.digests {
		if subtle.ConstantTimeCompare(sum[:], digest) == 1 {
			matched = i
		}
	}
	if matched < 0 {
		return "", false
	}
	return set.fingerprints[matched], true
}

// PublishKeysConfigured reports whether at least one publish key is active.
func PublishKeysConfigured() bool {
	set := publishKeyPtr.Load()
	return set != nil && len(set.digests) > 0
}

// CORSAllowOrigin returns the Access-Control-Allow-Origin value for a request
// Origin and whether that origin is allowed. An empty Origin (a non-browser
// client, or a same-origin request) is never answered with CORS headers, so an
// unconfigured server keeps sending no Access-Control-Allow-Origin at all.
func CORSAllowOrigin(origin string) (string, bool) {
	if origin == "" {
		return "", false
	}
	set := corsPtr.Load()
	if set == nil {
		return "", false
	}
	if set.wildcard {
		return "*", true
	}
	for _, allowed := range set.origins {
		if strings.EqualFold(allowed, origin) {
			return allowed, true
		}
	}
	return "", false
}

// CORSConfigured reports whether any origin is allowed. Preflight requests are
// answered only when it is true, so the default deployment keeps returning 405
// for OPTIONS exactly as before.
func CORSConfigured() bool {
	set := corsPtr.Load()
	return set != nil && (set.wildcard || len(set.origins) > 0)
}

// parseCORSOrigin normalizes one CORS_ORIGINS entry. Only "*" and an absolute
// http(s) origin without path, query, fragment or userinfo are accepted,
// because those are the only shapes a browser can send in an Origin header.
func parseCORSOrigin(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "*" {
		return "*", true
	}
	u, err := url.Parse(s)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", false
	}
	if strings.TrimSuffix(u.Path, "/") != "" {
		return "", false
	}
	host := strings.ToLower(u.Host)
	if host == "" {
		return "", false
	}
	return u.Scheme + "://" + host, true
}
