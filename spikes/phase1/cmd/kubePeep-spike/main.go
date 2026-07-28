package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/fvmoraes/ginger/pkg/app"
	"github.com/fvmoraes/ginger/pkg/config"
	"github.com/fvmoraes/kubepeep/spikes/phase1/spike"
	_ "modernc.org/sqlite"
)

func main() {
	signalContext, stopSignals := signalContext(context.Background())
	defer stopSignals()

	command := spike.NewRootCommand(run)
	if err := command.ExecuteContext(signalContext); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, options spike.StartOptions) error {
	frontend, err := spike.FrontendFS()
	if err != nil {
		return fmt.Errorf("embedded frontend: %w", err)
	}
	dataDirectory, err := os.MkdirTemp("", "kubePeep-phase1-")
	if err != nil {
		return fmt.Errorf("create spike data directory: %w", err)
	}
	defer os.RemoveAll(dataDirectory) //nolint:errcheck

	database, err := sql.Open("sqlite", filepath.Join(dataDirectory, "phase1.db"))
	if err != nil {
		return fmt.Errorf("open spike database: %w", err)
	}
	defer database.Close() //nolint:errcheck
	if err := spike.ApplyEmbeddedMigrations(ctx, database); err != nil {
		return err
	}
	var tableCount int
	if err := database.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'phase1_probe'`,
	).Scan(&tableCount); err != nil {
		return fmt.Errorf("verify embedded migration: %w", err)
	}
	if tableCount != 1 {
		return fmt.Errorf("embedded migration did not create phase1_probe")
	}

	application := app.New(&config.Config{
		App: config.AppConfig{Name: "kubePeep-phase1-spike", Env: "development"},
		HTTP: config.HTTPConfig{
			Host:            "127.0.0.1",
			Port:            options.Port,
			ShutdownTimeout: 2,
		},
		Log: config.LogConfig{Level: "info", Format: "json"},
	})
	application.Router.HandleRaw("GET /", spike.SPAHandler(frontend))

	localRuntime, err := spike.NewRuntime(application, options.Port, 50)
	if err != nil {
		return err
	}
	return localRuntime.RunWithReady(ctx, 3*time.Second, func(_ context.Context, url string) error {
		fmt.Printf("kubePeep phase 1 spike ready at %s; embedded migration applied\n", url)
		if options.NoBrowser {
			return nil
		}
		return openBrowser(url)
	})
}

func openBrowser(url string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", url)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		command = exec.Command("xdg-open", url)
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	return nil
}
