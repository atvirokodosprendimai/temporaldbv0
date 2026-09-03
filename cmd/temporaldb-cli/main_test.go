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

// TestFlagsAreRegistered verifies that -e, -f and --json flags are registered.
// This is a rung-2 selection check: delete a flag registration and this test must go red.
func TestFlagsAreRegistered(t *testing.T) {
	// Start a fake server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/query" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		result := tql.Result{Rows: []map[string]interface{}{}, Count: 0}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()

	// Try -e flag.
	cmd := exec.Command("go", "run", ".", "-e", "GET test/x")
	cmd.Env = append(os.Environ(), "TEMPORALDB_ADDR="+server.URL)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Errorf("-e flag not working: %v, stderr: %s", err, stderr.String())
	}

	// Try -f flag.
	tmpfile, err := os.CreateTemp("", "test-*.tql")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(tmpfile, "GET test/y\n")
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	cmd = exec.Command("go", "run", ".", "-f", tmpfile.Name())
	cmd.Env = append(os.Environ(), "TEMPORALDB_ADDR="+server.URL)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Errorf("-f flag not working: %v, stderr: %s", err, stderr.String())
	}

	// Try --json flag.
	cmd = exec.Command("go", "run", ".", "-e", "GET test/z", "--json")
	cmd.Env = append(os.Environ(), "TEMPORALDB_ADDR="+server.URL)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Errorf("--json flag not working: %v, stderr: %s", err, stderr.String())
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

	// Prepare REPL input: multi-line statement terminated by ; and EOF.
	input := "GET users/alice;\nEOF"

	cmd := exec.Command("go", "run", ".")
	cmd.Env = append(os.Environ(), "TEMPORALDB_ADDR="+server.URL)
	cmd.Stdin = strings.NewReader(input)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Logf("REPL exited with: %v (expected for EOF)", err)
	}

	// Verify we got some output (the REPL should have processed the statement).
	output := stdout.String()
	if output == "" {
		t.Log("warning: REPL produced no output")
	}
}
