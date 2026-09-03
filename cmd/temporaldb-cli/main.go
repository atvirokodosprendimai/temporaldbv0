// Package main provides the temporaldb CLI tool.
// It offers an interactive REPL, one-shot execution via -e, and batch execution via -f (D11).
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/urfave/cli/v3"
	"oneshottest/client"
)

// main is the entry point for the CLI.
func main() {
	cmd := &cli.Command{
		Name:  "temporaldb-cli",
		Usage: "Temporal database CLI",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "e",
				Usage: "execute a single TQL statement",
			},
			&cli.StringFlag{
				Name:  "f",
				Usage: "execute statements from a file",
			},
			&cli.BoolFlag{
				Name:  "json",
				Usage: "output as JSON",
			},
		},
		Action: run,
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// run is the main action handler for the CLI.
func run(ctx context.Context, cmd *cli.Command) error {
	addr := os.Getenv("TEMPORALDB_ADDR")
	if addr == "" {
		addr = "http://localhost:7777"
	}

	c := client.New(addr)
	forceJSON := cmd.Bool("json")
	isTTY := isTerminal(os.Stdout.Fd())
	r := newRenderer(isTTY && !forceJSON)

	// Determine mode: -e (one shot), -f (file), or REPL (interactive).
	if e := cmd.String("e"); e != "" {
		// One-shot mode: execute one statement.
		return executeStatement(ctx, c, e, r)
	}

	if f := cmd.String("f"); f != "" {
		// File mode: execute statements from a file.
		return executeFile(ctx, c, f, r)
	}

	// REPL mode: interactive.
	return repl(ctx, c, r)
}

// executeStatement executes a single TQL statement.
func executeStatement(ctx context.Context, c *client.Client, stmt string, r *renderer) error {
	result, err := c.Query(ctx, stmt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "query error: %v\n", err)
		return err
	}

	if result.Error != "" {
		fmt.Fprintf(os.Stderr, "query error: %s\n", result.Error)
		return fmt.Errorf("query error: %s", result.Error)
	}

	// Render the result.
	if err := r.render(result); err != nil {
		fmt.Fprintf(os.Stderr, "render error: %v\n", err)
		return err
	}

	return nil
}

// executeFile executes all TQL statements from a file, one per line.
// Stops at the first error and returns non-zero exit code.
func executeFile(ctx context.Context, c *client.Client, filename string, r *renderer) error {
	file, err := os.Open(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "file error: %v\n", err)
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		stmt := strings.TrimSpace(scanner.Text())
		if stmt == "" || strings.HasPrefix(stmt, "#") {
			// Skip empty lines and comments.
			continue
		}

		result, err := c.Query(ctx, stmt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "query error: %v\n", err)
			return err
		}

		if result.Error != "" {
			fmt.Fprintf(os.Stderr, "query error: %s\n", result.Error)
			return fmt.Errorf("query error: %s", result.Error)
		}

		// Render the result.
		if err := r.render(result); err != nil {
			fmt.Fprintf(os.Stderr, "render error: %v\n", err)
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "file read error: %v\n", err)
		return err
	}

	return nil
}

// isTerminal checks if the given file descriptor is a TTY.
func isTerminal(fd uintptr) bool {
	return isatty.IsTerminal(fd)
}
