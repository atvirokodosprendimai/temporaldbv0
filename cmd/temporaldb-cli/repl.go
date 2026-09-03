package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"oneshottest/client"
)

// repl runs the interactive REPL loop.
func repl(ctx context.Context, c *client.Client, r *renderer) error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("TemporalDB CLI - type TQL commands or \\q to quit")

	for {
		// Prompt for input.
		fmt.Print("> ")

		// Read until we get a complete statement (ends with ; or \q).
		var lines []string
		for {
			line, err := reader.ReadString('\n')
			if err != nil && err != io.EOF {
				fmt.Fprintf(os.Stderr, "read error: %v\n", err)
				return err
			}

			line = strings.TrimSpace(line)

			// Check for exit command.
			if strings.ToLower(line) == "\\q" || strings.ToLower(line) == "exit" {
				return nil
			}

			lines = append(lines, line)

			// Check if statement is complete (ends with ;).
			stmt := strings.Join(lines, " ")
			if strings.HasSuffix(stmt, ";") {
				// Remove trailing semicolon.
				stmt = strings.TrimSuffix(stmt, ";")
				stmt = strings.TrimSpace(stmt)

				// Execute the statement.
				result, err := c.Query(ctx, stmt)
				if err != nil {
					fmt.Fprintf(os.Stderr, "query error: %v\n", err)
					break // Show prompt again on error
				}

				if result.Error != "" {
					fmt.Fprintf(os.Stderr, "query error: %s\n", result.Error)
					break // Show prompt again on error
				}

				// Render the result.
				if err := r.render(result); err != nil {
					fmt.Fprintf(os.Stderr, "render error: %v\n", err)
				}
				break // Back to prompt
			}

			// EOF without semicolon: exit the REPL.
			if err == io.EOF {
				return nil
			}

			// Prompt for continuation.
			fmt.Print("  ")
		}
	}
}
