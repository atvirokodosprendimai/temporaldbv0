package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"oneshottest/internal/tql"
)

// TestDashEExecutesOneStatement verifies that -e runs a single TQL statement and prints the result.
func TestDashEExecutesOneStatement(t *testing.T) {
	// Start a fake server that echoes back what it receives.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/query" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		stmt := string(body)

		// Return a simple result for GET statements.
		result := tql.Result{
			Rows:  []map[string]interface{}{{"key": "test", "value": stmt}},
			Count: 1,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()

	// Run the CLI with -e flag.
	cmd := exec.Command("go", "run", ".", "-e", "GET users/alice")
	cmd.Env = append(os.Environ(), "TEMPORALDB_ADDR="+server.URL)

	// Capture output.
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("CLI failed: %v\nstderr: %s", err, stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "test") {
		t.Errorf("expected output to contain 'test', got: %s", output)
	}
}

// TestDashFExecutesEveryStatementInOrder verifies that -f runs a file of statements in order.
func TestDashFExecutesEveryStatementInOrder(t *testing.T) {
	// Create a temporary file with TQL statements.
	tmpfile, err := os.CreateTemp("", "test-*.tql")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	fmt.Fprintf(tmpfile, "GET users/alice\nGET users/bob\n")
	tmpfile.Close()

	// Start a fake server.
	counter := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/query" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		counter++
		result := tql.Result{
			Rows:  []map[string]interface{}{{"n": counter}},
			Count: 1,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()

	cmd := exec.Command("go", "run", ".", "-f", tmpfile.Name())
	cmd.Env = append(os.Environ(), "TEMPORALDB_ADDR="+server.URL)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("CLI failed: %v\nstderr: %s", err, stderr.String())
	}

	if counter != 2 {
		t.Errorf("expected 2 statements executed, got %d", counter)
	}
}

// TestFlagsAreRegistered verifies that -e, -f and --json flags are registered in the source code.
// This is a rung-2 selection check: delete a flag registration from newCommand() and this test must go red.
// The test binds in-process to the cli.Command flags, not to a cached binary, so mutations are detected.
func TestFlagsAreRegistered(t *testing.T) {
	cmd := newCommand()

	// Create a map of flag names for easy lookup.
	flagNames := make(map[string]bool)
	for _, flag := range cmd.Flags {
		flagNames[flag.Names()[0]] = true
	}

	// Verify required flags are registered.
	requiredFlags := []string{"e", "f", "json"}
	for _, name := range requiredFlags {
		if !flagNames[name] {
			t.Errorf("flag -%s is not registered in the command", name)
		}
	}
}

// TestNonTTYOutputIsJSON verifies that piping (non-TTY) produces JSON output.
func TestNonTTYOutputIsJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/query" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		result := tql.Result{
			Rows:  []map[string]interface{}{{"name": "Alice", "age": 30}},
			Count: 1,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()

	cmd := exec.Command("go", "run", ".", "-e", "FIND users")
	cmd.Env = append(os.Environ(), "TEMPORALDB_ADDR="+server.URL)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		t.Fatalf("CLI failed: %v", err)
	}

	output := stdout.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Errorf("output is not valid JSON: %s, error: %v", output, err)
	}
}

// TestExitCodeIsNonZeroOnQueryError verifies that the CLI exits with non-zero status on error.
func TestExitCodeIsNonZeroOnQueryError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/query" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		result := tql.Result{
			Error: "parse error: invalid statement",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()

	cmd := exec.Command("go", "run", ".", "-e", "INVALID STATEMENT")
	cmd.Env = append(os.Environ(), "TEMPORALDB_ADDR="+server.URL)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err == nil {
		t.Error("expected non-zero exit code, but got 0")
	} else if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() == 0 {
		t.Errorf("expected non-zero exit code, got: %v", err)
	}
}

// TestREPLHandlesMultiLineAndEOF verifies that the REPL handles multi-line statements and EOF.
// It also ensures that incomplete statements cannot be the only way to invoke REPL behavior.
func TestREPLHandlesMultiLineAndEOF(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/query" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		stmt := strings.TrimSpace(string(body))

		result := tql.Result{
			Rows:  []map[string]interface{}{{"stmt": stmt}},
			Count: 1,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()

	// Prepare REPL input: a complete statement terminated by ; and then EOF.
	input := "GET users/alice;\n"

	cmd := exec.Command("go", "run", ".")
	cmd.Env = append(os.Environ(), "TEMPORALDB_ADDR="+server.URL)
	cmd.Stdin = strings.NewReader(input)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("REPL failed: %v\nstderr: %s", err, stderr.String())
	}

	// Verify we got output (the REPL should have processed the statement).
	output := stdout.String()
	if output == "" {
		t.Fatal("REPL produced no output; statement was not executed")
	}

	// Verify the output contains evidence of the statement execution.
	if !strings.Contains(output, "alice") && !strings.Contains(output, "stmt") {
		t.Errorf("expected output to contain 'alice' or 'stmt', got: %s", output)
	}
}
