package main

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
)

//go:embed front-end/out/*
var frontendFS embed.FS

// Get the backend binary name based on the platform
func getBackendBinaryName() string {
	binary := "./test"
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	return binary
}

// Start the backend process
func startBackend() (*exec.Cmd, error) {
	log.Println("Starting backend process...")
	cmd := exec.Command(getBackendBinaryName())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	if err != nil {
		return nil, err
	}
	return cmd, nil
}

// File server that serves everything in the embedded folder
func startServer() (*http.ServeMux, error) {
	fsys, err := fs.Sub(frontendFS, "front-end/out")
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()

	// Serve all static files using http.FileServer
	fileServer := http.FileServer(http.FS(fsys))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Try to serve the requested file
		if fileExists(fsys, r.URL.Path) {
			fileServer.ServeHTTP(w, r)
		} else {
			// If the file doesn't exist, serve index.html for client-side routing
			f, err := fsys.Open("index.html")
			if err != nil {
				http.Error(w, "index.html not found", http.StatusNotFound)
				return
			}
			defer f.Close()

			// Read the content of index.html
			content, err := io.ReadAll(f)
			if err != nil {
				http.Error(w, "failed to read index.html", http.StatusInternalServerError)
				return
			}

			// Serve index.html using bytes.Reader which implements io.ReadSeeker
			http.ServeContent(w, r, "index.html", time.Now(), bytes.NewReader(content))
		}
	})

	log.Println("Frontend server is set up to serve all files in the embedded folder.")

	return mux, nil
}

// Check if a file exists in the embedded filesystem
func fileExists(fsys fs.FS, filePath string) bool {
	if filePath == "/" {
		filePath = "index.html" // Serve index.html if root is requested
	}

	// Attempt to open the file
	_, err := fsys.Open(strings.TrimPrefix(filePath, "/"))
	return err == nil
}

// StartHTTPServer starts the HTTP server with graceful shutdown
func startHTTPServer(server *http.Server) {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	log.Println("HTTP server is running on", server.Addr)

	<-stop

	log.Println("Shutting down HTTP server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("HTTP server graceful shutdown failed: %v", err)
	}
	log.Println("HTTP server stopped.")
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Start backend process
	backendCmd, err := startBackend()
	if err != nil {
		log.Fatalf("Failed to start backend: %v", err)
	}
	defer func() {
		// Ensure backend process is stopped when the application shuts down
		if backendCmd != nil && backendCmd.Process != nil {
			backendCmd.Process.Kill()
		}
	}()

	// Setup frontend server
	mux, err := startServer()
	if err != nil {
		log.Fatalf("Failed to start frontend server: %v", err)
	}

	// Create HTTP server with mux and address
	server := &http.Server{
		Addr:    fmt.Sprintf(":%s", port),
		Handler: mux,
	}

	// Start HTTP server with graceful shutdown
	startHTTPServer(server)
}
