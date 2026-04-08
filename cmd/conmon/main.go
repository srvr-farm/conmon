package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/mcallan/conmon/internal/app"
)

func main() {
	configPath := flag.String("config", "/etc/conmon/config.yml", "Path to the conmon configuration")
	flag.Parse()

	instance, err := app.New(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build app: %v\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := instance.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "conmon exited with error: %v\n", err)
		os.Exit(1)
	}
}
