package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/mcallan/conmon/internal/sysmon/app"
	"github.com/mcallan/conmon/internal/sysmon/config"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

func run(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("sysmon", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "/etc/sysmon/config.yml", "Path to the sysmon configuration")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "failed to load config: %v\n", err)
		return 1
	}

	instance, err := app.New(cfg, os.Hostname)
	if err != nil {
		fmt.Fprintf(stderr, "failed to build app: %v\n", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := instance.Run(ctx); err != nil {
		fmt.Fprintf(stderr, "sysmon exited with error: %v\n", err)
		return 1
	}

	return 0
}
