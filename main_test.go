package main

import (
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

// Test if the frontend static files are served correctly
func TestFrontendServe(t *testing.T) {
	// Initialize the server mux
	mux, err := startServer()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Create a test server
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Make a test request to the root endpoint
	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("Failed to send GET request: %v", err)
	}
	defer res.Body.Close()

	// Verify the response status code
	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status code 200, but got %d", res.StatusCode)
	}

	// Check the Content-Type to ensure it's serving static HTML
	if contentType := res.Header.Get("Content-Type"); !strings.Contains(contentType, "text/html") {
		t.Errorf("Expected Content-Type to be text/html, but got %s", contentType)
	}
}

// Test if the backend process starts successfully
func TestBackendServe(t *testing.T) {
	// Mock the backend command
	cmd := exec.Command("echo", "Backend running")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run mock backend: %v", err)
	}

	// Verify the output contains expected message
	if !strings.Contains(string(output), "Backend running") {
		t.Errorf("Expected backend to print 'Backend running', but got %s", string(output))
	}
}

// Integration test to ensure frontend and backend work together
func TestIntegration(t *testing.T) {
	// Start backend (mock)
	go func() {
		if err := startBackend(); err != nil {
			t.Fatalf("Failed to start backend: %v", err)
		}
	}()

	// Start frontend server
	mux, err := startServer()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Test server instance
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Check if the frontend is being served
	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("Failed to send GET request: %v", err)
	}
	defer res.Body.Close()

	// Check response status
	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status code 200, but got %d", res.StatusCode)
	}
}
