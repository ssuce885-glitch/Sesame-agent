package main

import (
	"context"
	"fmt"
	"os"

	"go-agent/internal/cli"
)

func main() {
	ctx := context.Background()
	args := os.Args[1:]
	if err := cli.New().Run(ctx, args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
