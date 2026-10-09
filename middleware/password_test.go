// SPDX-License-Identifier: MIT

package middleware

import (
	"strings"
	"testing"

	"lanpaper/config"
)

func TestArgon2PasswordHash(t *testing.T) {
	hash, err := GeneratePasswordHash("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Fatalf("unexpected hash format: %q", hash)
	}
	if !verifyPasswordHash("correct horse battery staple", hash) {
		t.Fatal("correct password rejected")
	}
	if verifyPasswordHash("wrong", hash) {
		t.Fatal("wrong password accepted")
	}
	if verifyPasswordHash("anything", "$argon2id$v=19$m=999999999,t=3,p=2$AA$AA") {
		t.Fatal("unsafe parameters accepted")
	}
}

func TestPasswordHashTakesPrecedenceOverPlaintext(t *testing.T) {
	hash, err := GeneratePasswordHash("new password")
	if err != nil {
		t.Fatal(err)
	}
	old := config.Current
	t.Cleanup(func() { config.Current = old })
	config.Current.AdminPass = "old password"
	config.Current.AdminPasswordHash = hash
	if !configuredPasswordOK("new password") {
		t.Fatal("hashed password rejected")
	}
	if configuredPasswordOK("old password") {
		t.Fatal("plaintext fallback bypassed configured hash")
	}
}

func TestCredentialFingerprintChangesOnRotation(t *testing.T) {
	old := config.Current
	t.Cleanup(func() { config.Current = old })
	config.Current.AdminPasswordHash = "first"
	first := credentialFingerprint()
	config.Current.AdminPasswordHash = "second"
	if first == credentialFingerprint() {
		t.Fatal("rotation did not change session credential fingerprint")
	}
}
