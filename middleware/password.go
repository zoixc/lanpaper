// SPDX-License-Identifier: MIT

package middleware

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"lanpaper/config"

	"golang.org/x/crypto/argon2"
)

const (
	argonMemory  = 64 * 1024
	argonTime    = 3
	argonThreads = 2
	argonKeyLen  = 32
)

// dummyPasswordHash is computed on first use, not at package init: it costs
// argonMemory (64 MiB) of heap, and only the plaintext-migration mode needs it.
// A process that uses a configured hash never allocates it.
var (
	dummyOnce         sync.Once
	dummyPasswordHash string
)

// dummyHash returns a fixed Argon2id hash used to keep timing equal when no
// real hash is configured. Its fixed salt is intentional: it authenticates
// nobody.
func dummyHash() string {
	dummyOnce.Do(func() {
		dummyPasswordHash = encodePasswordHash("invalid-password", []byte("lanpaper-dummy!!"))
	})
	return dummyPasswordHash
}

func GeneratePasswordHash(password string) (string, error) {
	if password == "" {
		return "", errors.New("password must not be empty")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	return encodePasswordHash(password, salt), nil
}

func encodePasswordHash(password string, salt []byte) string {
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
}

func verifyPasswordHash(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	var memory uint64
	var iterations uint64
	var threads uint64
	for _, item := range strings.Split(parts[3], ",") {
		kv := strings.SplitN(item, "=", 2)
		if len(kv) != 2 {
			return false
		}
		n, err := strconv.ParseUint(kv[1], 10, 32)
		if err != nil {
			return false
		}
		switch kv[0] {
		case "m":
			memory = n
		case "t":
			iterations = n
		case "p":
			threads = n
		default:
			return false
		}
	}
	// Bound attacker-controlled configuration before allocating memory.
	if memory < 8*1024 || memory > 1024*1024 || iterations < 1 || iterations > 10 || threads < 1 || threads > 16 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 8 || len(salt) > 64 {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) < 16 || len(want) > 64 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, uint32(iterations), uint32(memory), uint8(threads), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func configuredPasswordOK(password string) bool {
	if configHash := config.Current.AdminPasswordHash; configHash != "" {
		return verifyPasswordHash(password, configHash)
	}
	// Plaintext migration mode still performs a full Argon2id calculation, so
	// timing does not disclose which credential storage mode is configured.
	_ = verifyPasswordHash(password, dummyHash())
	return secureCompare(password, config.Current.AdminPass)
}

func credentialFingerprint() string {
	material := config.Current.AdminPasswordHash
	if material == "" {
		material = config.Current.AdminPass
	}
	sum := sha256.Sum256([]byte(material))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
