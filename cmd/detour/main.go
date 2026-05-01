package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/trydydd/detour/internal/config"
	"github.com/trydydd/detour/internal/launcher"
	"github.com/trydydd/detour/internal/proxy"
)

func main() {
	// --- Flags ---
	flags := &config.Config{}
	flag.StringVar(&flags.ModelName, "model-name", "", "alias sent as model name to Claude Code (required)")
	flag.StringVar(&flags.ModelAPI, "model-api", "", "base URL of local inference server, e.g. http://192.168.0.28 (required)")
	flag.IntVar(&flags.Port, "port", 0, "proxy listen port (default 8888)")
	flag.BoolVar(&flags.NoHaiku, "no-haiku", false, "skip overriding haiku model tier")
	flag.Parse()
	claudeArgs := flag.Args()

	// --- Config: load saved, merge flags ---
	cfgDir := detourDir()
	saved, err := config.Load(cfgDir)
	if err != nil {
		fatalf("load config: %v", err)
	}
	cfg := config.MergeFlags(saved, flags)
	cfg.ApplyDefaults()

	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, "detour:", err)
		fmt.Fprintln(os.Stderr, "Usage: detour --model-name <alias> --model-api <url> [-- claude args]")
		os.Exit(1)
	}

	// Save updated config for next run.
	if err := cfg.Save(cfgDir); err != nil {
		fmt.Fprintf(os.Stderr, "detour: warning: could not save config: %v\n", err)
	}

	// --- Start proxy ---
	proxyCfg := &proxy.Config{
		ModelName:            cfg.ModelName,
		LocalUpstreamURL:     cfg.ModelAPI,
		AnthropicUpstreamURL: "https://api.anthropic.com",
	}
	mux := proxy.NewMux(proxyCfg)
	addr := fmt.Sprintf(":%d", cfg.Port)
	srv := &http.Server{Addr: addr, Handler: mux}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintln(os.Stderr, "detour: proxy error:", err)
		}
	}()

	if err := waitForPort(addr, 3*time.Second); err != nil {
		fatalf("proxy did not start: %v", err)
	}
	fmt.Fprintf(os.Stderr, "detour: proxy on %s  [%s → local | opus → anthropic]\n", addr, cfg.ModelName)

	// --- Launch claude (or just serve if no claude found) ---
	launchErr := launcher.Launch(cfg, claudeArgs, "")
	if launchErr != nil {
		fmt.Fprintln(os.Stderr, "detour:", launchErr)
	}

	// Shut down proxy.
	shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	srv.Shutdown(shutCtx)
	_ = ctx

	if launchErr != nil {
		os.Exit(1)
	}
}

func detourDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		fatalf("home dir: %v", err)
	}
	return filepath.Join(home, ".detour")
}

func waitForPort(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", addr)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "detour: "+format+"\n", args...)
	os.Exit(1)
}
