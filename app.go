// SPDX-License-Identifier: MIT

package main

import (
	"log/slog"
	"net/http"

	"lanpaper/config"
	"lanpaper/middleware"
	"lanpaper/storage"
)

type AppServices struct {
	Wallpapers *storage.Store
}

// App is the composition root. Configuration is copied at construction so the
// dependency description is immutable even while compatibility handlers still
// read config.Current. Later extraction PRs replace those adapters one service
// at a time without changing the route contract.
type App struct {
	Config   config.Config
	Sessions *middleware.SessionStore
	Limiters *middleware.RateStore
	Logger   *slog.Logger
	Services AppServices
}

func NewApp() *App {
	return &App{Config: config.Current, Sessions: middleware.NewSessionStore(), Limiters: middleware.NewRateStore(), Logger: slog.Default(), Services: AppServices{Wallpapers: storage.Global}}
}

func (a *App) Handler() http.Handler {
	return middleware.Metrics(middleware.Gzip(middleware.Recover(a.mux())))
}
