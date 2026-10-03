// SPDX-License-Identifier: MIT

package config

import (
	"encoding/json"
	"strings"
	"testing"
)

// withDerived restores both the config and the parsed snapshots, so a test that
// installs keys or origins cannot leak them into another test.
func withDerived(t *testing.T, mutate func(*Config)) {
	t.Helper()
	saved := Current
	mutate(&Current)
	RefreshDerived()
	t.Cleanup(func() {
		Current = saved
		RefreshDerived()
	})
}

func TestPublishKeysAreMatchedByDigestOnly(t *testing.T) {
	const first = "a-good-publish-key-0001"
	const second = "a-good-publish-key-0002"
	withDerived(t, func(cfg *Config) {
		// A short key and a duplicate are dropped, and neither is accepted.
		cfg.PublishKeys = []string{first, second, "short", first, "   "}
	})

	if !PublishKeysConfigured() {
		t.Fatal("configured keys were not parsed")
	}
	fingerprint, ok := MatchPublishKey(first)
	if !ok || len(fingerprint) != KeyFingerprintLen {
		t.Fatalf("first key: %q %v", fingerprint, ok)
	}
	if strings.Contains(first, fingerprint) {
		t.Fatalf("fingerprint %q is part of the key", fingerprint)
	}
	if again, ok := MatchPublishKey(first); !ok || again != fingerprint {
		t.Fatalf("fingerprint is not stable: %q %q", fingerprint, again)
	}
	if other, ok := MatchPublishKey(second); !ok || other == fingerprint {
		t.Fatalf("second key must have its own fingerprint: %q %v", other, ok)
	}
	for _, rejected := range []string{"", "short", "a-good-publish-key-0003", "A-GOOD-PUBLISH-KEY-0001",
		"a-good-publish-key-000", first + " ", " a-good-publish-key-0001"} {
		if fp, ok := MatchPublishKey(rejected); ok {
			t.Fatalf("%q was accepted as %s", rejected, fp)
		}
	}

	withDerived(t, func(cfg *Config) { cfg.PublishKeys = nil })
	if PublishKeysConfigured() {
		t.Fatal("keys survived being unset")
	}
	if _, ok := MatchPublishKey(first); ok {
		t.Fatal("a removed key still matches")
	}
}

func TestPublishKeysAreCappedAndNeverSerialized(t *testing.T) {
	keys := make([]string, 0, MaxPublishKeys+5)
	for i := range MaxPublishKeys + 5 {
		keys = append(keys, "publish-key-number-"+strings.Repeat("0", i))
	}
	withDerived(t, func(cfg *Config) { cfg.PublishKeys = keys })
	if !PublishKeysConfigured() {
		t.Fatal("no key was accepted")
	}
	// Only the first MaxPublishKeys entries are usable.
	if _, ok := MatchPublishKey(keys[MaxPublishKeys]); ok {
		t.Fatal("a key beyond the limit was accepted")
	}
	if _, ok := MatchPublishKey(keys[MaxPublishKeys-1]); !ok {
		t.Fatal("a key inside the limit was rejected")
	}

	// A secret must never reach config.json, however the struct is marshalled.
	withDerived(t, func(cfg *Config) { cfg.PublishKeys = []string{"super-secret-publish-key"} })
	body, err := json.Marshal(Current)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "super-secret-publish-key") {
		t.Fatalf("the publish key was serialized: %s", body)
	}
}

func TestCORSOriginsAreNormalizedAndMatchedExactly(t *testing.T) {
	withDerived(t, func(cfg *Config) {
		cfg.CORSOrigins = []string{
			"https://frame.example",
			"http://192.168.1.5:8080",
			"HTTPS://Upper.Example/",
			"https://evil.example/path",
			"https://user:pass@vault.example",
			"ftp://files.example",
			"frame.example",
			"",
		}
	})
	if !CORSConfigured() {
		t.Fatal("valid origins were not parsed")
	}
	for _, origin := range []string{"https://frame.example", "http://192.168.1.5:8080", "https://upper.example"} {
		if value, ok := CORSAllowOrigin(origin); !ok || value != origin {
			t.Fatalf("%s: %q %v", origin, value, ok)
		}
	}
	for _, origin := range []string{"", "https://evil.example", "http://frame.example", "https://frame.example:443",
		"https://vault.example", "https://files.example", "https://not-listed.example"} {
		if value, ok := CORSAllowOrigin(origin); ok {
			t.Fatalf("%s must not be allowed, got %q", origin, value)
		}
	}

	withDerived(t, func(cfg *Config) { cfg.CORSOrigins = []string{"https://one.example", "*"} })
	if value, ok := CORSAllowOrigin("https://anywhere.example"); !ok || value != "*" {
		t.Fatalf("wildcard: %q %v", value, ok)
	}

	withDerived(t, func(cfg *Config) { cfg.CORSOrigins = nil })
	if CORSConfigured() {
		t.Fatal("CORS stayed configured after the list was cleared")
	}
	if _, ok := CORSAllowOrigin("https://anywhere.example"); ok {
		t.Fatal("an origin was allowed without configuration")
	}
}

func TestEnvListSplitsAndKeepsPreviousValue(t *testing.T) {
	t.Setenv("LANPAPER_TEST_LIST", " a , ,b,")
	var list []string
	envList("LANPAPER_TEST_LIST", &list)
	if len(list) != 2 || list[0] != "a" || list[1] != "b" {
		t.Fatalf("envList = %#v", list)
	}
	// An unset or blank variable keeps the previous value, like envInt/envBool.
	t.Setenv("LANPAPER_TEST_LIST", "   ")
	envList("LANPAPER_TEST_LIST", &list)
	if len(list) != 2 {
		t.Fatalf("a blank variable replaced the list: %#v", list)
	}
	t.Setenv("LANPAPER_TEST_LIST", "c")
	envList("LANPAPER_TEST_LIST", &list)
	if len(list) != 1 || list[0] != "c" {
		t.Fatalf("envList did not replace the list: %#v", list)
	}
}

func TestValidateClampsHistoryAndPlaylist(t *testing.T) {
	saved := Current
	t.Cleanup(func() { Current = saved; RefreshDerived() })

	Current = Config{Port: "8080", MaxUploadMB: DefaultMaxUploadMB,
		MaxConcurrentUploads: DefaultMaxConcurrentUploads, MaxWalkDepth: DefaultMaxWalkDepth,
		History: HistoryConfig{Limit: MaxHistoryLimit + 1, MaxMB: -1}}
	validate()
	if Current.History.Limit != DefaultHistoryLimit || Current.History.MaxMB != DefaultHistoryMaxMB {
		t.Fatalf("out-of-range history was not clamped: %+v", Current.History)
	}
	if Current.PlaylistMax != DefaultPlaylistMax {
		t.Fatalf("playlistMax = %d, want %d", Current.PlaylistMax, DefaultPlaylistMax)
	}

	// 0 is a deliberate choice, not a typo: it turns the features off.
	Current.History = HistoryConfig{Limit: 0, MaxMB: 0}
	Current.PlaylistMax = 4
	validate()
	if Current.History.Limit != 0 || Current.History.MaxMB != 0 || Current.PlaylistMax != 4 {
		t.Fatalf("explicit zeros were rewritten: %+v %d", Current.History, Current.PlaylistMax)
	}
}
