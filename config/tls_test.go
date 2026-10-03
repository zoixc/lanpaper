// SPDX-License-Identifier: MIT

package config

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestTLSRequiresCertificateAndKeyTogether(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		cert, key              string
		enabled, misconfigured bool
	}{
		{"unset", "", "", false, false},
		{"both set", "fullchain.pem", "privkey.pem", true, false},
		{"certificate only", "fullchain.pem", "", false, true},
		{"key only", "", "privkey.pem", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			Current = Config{TLSCertFile: tc.cert, TLSKeyFile: tc.key}
			if got := TLSEnabled(); got != tc.enabled {
				t.Errorf("TLSEnabled() = %v, want %v", got, tc.enabled)
			}
			if got := TLSMisconfigured(); got != tc.misconfigured {
				t.Errorf("TLSMisconfigured() = %v, want %v", got, tc.misconfigured)
			}
			// validate must not touch either field: refusing to start is main's
			// job, and silently dropping a certificate would serve plaintext.
			validate()
			if Current.TLSCertFile != tc.cert || Current.TLSKeyFile != tc.key {
				t.Errorf("validate changed the TLS settings: %q / %q", Current.TLSCertFile, Current.TLSKeyFile)
			}
		})
	}
}

func TestLoadReadsTLSFromEnvironment(t *testing.T) {
	t.Setenv("TLS_CERT_FILE", "/etc/lanpaper/fullchain.pem")
	t.Setenv("TLS_KEY_FILE", "/etc/lanpaper/privkey.pem")
	Load()
	if !TLSEnabled() {
		t.Fatalf("TLS_CERT_FILE/TLS_KEY_FILE were not picked up: %+v", Current)
	}
	if Current.TLSCertFile != "/etc/lanpaper/fullchain.pem" || Current.TLSKeyFile != "/etc/lanpaper/privkey.pem" {
		t.Fatalf("TLS paths = %q / %q", Current.TLSCertFile, Current.TLSKeyFile)
	}
}

func TestTLSPathsAreNotSecretsButKeysAre(t *testing.T) {
	// The certificate paths may live in config.json; the publish keys may not.
	Current = Config{TLSCertFile: "cert.pem", TLSKeyFile: "key.pem", PublishKeys: []string{"s3cret"}}
	body, err := json.Marshal(Current)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte(`"tlsCertFile":"cert.pem"`)) {
		t.Fatalf("TLS certificate path was not serialized: %s", body)
	}
	if bytes.Contains(body, []byte("s3cret")) || bytes.Contains(body, []byte("publishKeys")) {
		t.Fatalf("a publish key was serialized: %s", body)
	}
}
