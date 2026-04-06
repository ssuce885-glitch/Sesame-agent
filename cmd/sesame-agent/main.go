package main

import (
	"context"
	"fmt"
	"os"

	"go-agent/internal/cli"
)

func main() {
	app := cli.New()
	if err := app.Run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
