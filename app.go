// SPDX-License-Identifier: MIT

package main

import (
	"log/slog"
	"net/http"
	"time"

	"lanpaper/config"
	"lanpaper/middleware"
	"lanpaper/storage"
)

// SessionRuntime and LimiterRuntime are the narrow lifecycle views needed by
// application composition. Request-level injection is completed incrementally;
// the default adapters preserve the existing package API in the meantime.
type SessionRuntime interface{ ActiveCount() int }
type LimiterRuntime interface{ Clean(time.Time) }

type defaultSessions struct{}

func (defaultSessions) ActiveCount() int { return middleware.ActiveSessionCount() }

type defaultLimiters struct{}

func (defaultLimiters) Clean(now time.Time) { middleware.CleanRateLimits(now) }

type AppServices struct {
	Wallpapers *storage.Store
}

// App is the composition root. Configuration is copied at construction so the
// dependency description is immutable even while compatibility handlers still
// read config.Current. Later extraction PRs replace those adapters one service
// at a time without changing the route contract.
type App struct {
	Config   config.Config
	Sessions SessionRuntime
	Limiters LimiterRuntime
	Logger   *slog.Logger
	Services AppServices
}

func NewApp() *App {
	return &App{Config: config.Current, Sessions: defaultSessions{}, Limiters: defaultLimiters{}, Logger: slog.Default(), Services: AppServices{Wallpapers: storage.Global}}
}

func (a *App) Handler() http.Handler {
	return middleware.Metrics(middleware.Gzip(middleware.Recover(a.mux())))
}
