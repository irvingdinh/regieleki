package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "access-token" {
		handleAccessToken(os.Args[2:])
		return
	}

	dnsAddr := flag.String("dns", ":53", "DNS listen address")
	httpAddr := flag.String("http", ":13860", "HTTP listen address")
	dataPath := flag.String("data", "records.tsv", "Path to records file")
	tokenPath := flag.String("token", "", "Path to API token file (empty to disable auth)")
	debug := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()

	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	store, err := NewStore(*dataPath)
	if err != nil {
		slog.Error("failed to load store", "error", err)
		os.Exit(1)
	}
	slog.Info("store loaded", "records", len(store.List()), "path", *dataPath)

	var token string
	if *tokenPath != "" {
		token, err = loadOrCreateToken(*tokenPath)
		if err != nil {
			slog.Error("failed to load token", "error", err)
			os.Exit(1)
		}
		slog.Info("api token loaded", "path", *tokenPath)
	}

	upstreams := parseResolvConf()

	dns := NewDNSServer(store, upstreams)
	web := NewWebServer(store, token)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errc := make(chan error, 2)
	go func() { errc <- dns.ListenAndServe(*dnsAddr) }()
	go func() { errc <- web.ListenAndServe(*httpAddr) }()

	select {
	case err := <-errc:
		slog.Error("server error", "error", err)
		os.Exit(1)
	case <-ctx.Done():
		slog.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		web.Shutdown(shutdownCtx)
		dns.Close()
	}
}
