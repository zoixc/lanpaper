// SPDX-License-Identifier: MIT

package middleware

import (
	"crypto/sha256"
	"testing"
	"time"
)

func TestSessionStoresAreInstanceIsolated(t *testing.T) {
	now := time.Now()
	first := NewSessionStore()
	second := NewSessionStore()
	first.expiry[sha256.Sum256([]byte("one"))] = sessionRecord{Expires: now.Add(time.Hour)}
	if first.ActiveCount(now) != 1 {
		t.Fatal("first store lost session")
	}
	if second.ActiveCount(now) != 0 {
		t.Fatal("session leaked into second store")
	}
}

func TestRateStoresAreInstanceIsolated(t *testing.T) {
	first := NewRateStore()
	second := NewRateStore()
	if ok, _ := first.Allow("login", "client", 1, 1, time.Hour); !ok {
		t.Fatal("first token rejected")
	}
	if ok, _ := first.Allow("login", "client", 1, 1, time.Hour); ok {
		t.Fatal("first store exceeded capacity")
	}
	if ok, _ := second.Allow("login", "client", 1, 1, time.Hour); !ok {
		t.Fatal("bucket leaked into second store")
	}
}
