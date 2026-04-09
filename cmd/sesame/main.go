package main

import (
	"context"
	"fmt"
	"os"

	daemonapp "go-agent/internal/daemon"
	"go-agent/internal/cli"
)

func main() {
	ctx := context.Background()
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "daemon" {
		if err := daemonapp.Run(ctx); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := cli.New().Run(ctx, args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
