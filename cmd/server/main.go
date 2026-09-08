package main

import (
	"context"
	"errors"
	"m365-copilot2api/internal/applog"
	"m365-copilot2api/internal/outbound"
	"m365-copilot2api/internal/web"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	if exe, err := os.Executable(); err == nil {
		if dir := filepath.Dir(exe); dir != "" {
			os.Chdir(dir)
		}
	}
	if err := web.ApplyTimezoneEnv(); err != nil {
		applog.FatalError("server", "apply_timezone_failed", err)
	}
	web.ApplyStartupSettingsEnv()
	if err := outbound.ConfigureFromEnv(); err != nil {
		applog.FatalError("server", "configure_outbound_proxy_failed", err)
	}
	s, e := web.New()
	if e != nil {
		applog.FatalError("server", "initialize_web_server_failed", e)
	}
	s.InitM365CloudClient()
	s.StartAutoCleanup()
	s.StartConvCacheGC()
	s.RefreshExpiredTokens()
	s.PreheatPool()
	listen := "127.0.0.1:4141"
	if v := os.Getenv("M365_LISTEN"); v != "" {
		listen = v
	}
	applog.Info("server", "listening", "address", listen)
	server := &http.Server{
		Addr:              listen,
		Handler:           s.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
		WriteTimeout:      0, // streaming endpoints need an open-ended write window.
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			applog.Error("server", "graceful_shutdown_failed", "error", err)
		}
	}()
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		applog.FatalError("server", "serve_failed", err)
	}
	web.StopPersistLoop()
	applog.Info("server", "shutdown_complete")
}
