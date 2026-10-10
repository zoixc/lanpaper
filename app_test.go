// SPDX-License-Identifier: MIT

package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"lanpaper/config"
)

func TestAppCapturesImmutableConfigurationAndDependencies(t *testing.T) {
	old := config.Current
	t.Cleanup(func() { config.Current = old })
	config.Current.Port = "1111"
	app := NewApp()
	config.Current.Port = "2222"
	if app.Config.Port != "1111" {
		t.Fatalf("captured port=%q", app.Config.Port)
	}
	if app.Services.Wallpapers == nil || app.Sessions == nil || app.Limiters == nil || app.Logger == nil {
		t.Fatal("composition root has nil dependencies")
	}
}

func TestAppInstancesOwnIndependentRuntimeState(t *testing.T) {
	first, second := NewApp(), NewApp()
	if first.Sessions == second.Sessions || first.Limiters == second.Limiters {
		t.Fatal("application instances share mutable authentication or limiter state")
	}
}

func TestAppHandlerBuildsRealRouteStack(t *testing.T) {
	app := NewApp()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("health status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("security middleware missing from App.Handler")
	}
}
