// Copyright 2025 Toly Pochkin
// SPDX-License-Identifier: Apache-2.0

// Command gobetter generates and checks Go builders and related methods.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/mobiletoly/gobetter/internal/cli"
)

func main() {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Run(ctx, os.Args[1:], dir, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
