package cmd

import (
	_ "fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"text/template"

	"github.com/spf13/cobra"
)

// Template for the main.go file
const mainTemplate = `package main

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

//go:embed {{.EmbedPath}}
var frontendFS embed.FS

// Get the backend binary name based on the platform
func getBackendBinaryName() string {
	binary := "./backend-binary"
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
	fsys, err := fs.Sub(frontendFS, "{{.FrontendDir}}")
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

`

// RootCmd defines the base command for Cobra
var RootCmd = &cobra.Command{
	Use:   "GoNext <backend> <frontend> <output-dir> <binary-name>",
	Short: "GoNext CLI generates a Go web server from backend and frontend files",
	Args:  cobra.ExactArgs(4),
	Run:   run,
}

func run(cmd *cobra.Command, args []string) {
	backendPath := args[0]
	frontendPath := args[1]
	outputDir := args[2]
	outputBinary := filepath.Join(outputDir, args[3])

	// Add correct file extension based on the platform
	outputBinary = addPlatformExtension(outputBinary)

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("Backend path: %s", backendPath)
	log.Printf("Frontend path: %s", frontendPath)
	log.Printf("Output binary: %s", outputBinary)

	tempDir, err := os.MkdirTemp("", "gonext-")
	if err != nil {
		log.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)
	log.Printf("Created temp directory: %s", tempDir)

	// Build the Next.js frontend
	if err := buildNextJS(frontendPath); err != nil {
		log.Fatalf("Failed to build frontend: %v", err)
	}
	log.Println("Next.js frontend built successfully")

	// Copy only the built frontend (frontend/out)
	fullFrontendPath := filepath.Join(frontendPath, "out")
	destFrontendPath := filepath.Join(tempDir, filepath.Base(frontendPath))
	if err := copyDir(fullFrontendPath, destFrontendPath); err != nil {
		log.Fatalf("Failed to copy built frontend files: %v", err)
	}
	log.Println("Frontend files copied successfully")

	// Build the Go backend
	builtBackendBinary := filepath.Join(tempDir, "backend-binary")
	builtBackendBinary = addPlatformExtension(builtBackendBinary)
	if err := buildGoBackend(backendPath, builtBackendBinary); err != nil {
		log.Fatalf("Failed to build backend: %v", err)
	}
	log.Println("Go backend built successfully")

	// Copy the Go backend binary to the output directory
	if err := copyFile(builtBackendBinary, outputBinary); err != nil {
		log.Fatalf("Failed to copy backend binary to output: %v", err)
	}
	log.Printf("Backend binary copied to: %s", outputBinary)

	// Generate main.go
	mainFile := filepath.Join(tempDir, "main.go")
	if err := generateMain(mainFile, filepath.Base(frontendPath)); err != nil {
		log.Fatalf("Failed to generate main.go: %v", err)
	}
	log.Println("main.go generated successfully")

	// Initialize Go module
	if err := initGoModule(tempDir); err != nil {
		log.Fatalf("Failed to initialize Go module: %v", err)
	}

	// Build the final binary
	if err := buildBinary(tempDir, outputBinary); err != nil {
		log.Fatalf("Failed to build: %v", err)
	}
	log.Printf("Successfully created bundled binary: %s", outputBinary)
}

// Adds the correct file extension based on the platform
func addPlatformExtension(binary string) string {
	if runtime.GOOS == "windows" {
		return binary + ".exe"
	}
	return binary
}

func buildNextJS(frontendPath string) error {
	log.Println("Building Next.js frontend...")
	cmd := exec.Command("npm", "run", "build")
	cmd.Dir = frontendPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func buildGoBackend(backendPath, outputBinary string) error {
	log.Println("Building Go backend...")
	cmd := exec.Command("go", "build", "-o", outputBinary)
	cmd.Dir = backendPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(dstPath, data, info.Mode())
	})
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0755)
}

func generateMain(filename, frontendDir string) error {
	tmpl, err := template.New("main").Parse(mainTemplate)
	if err != nil {
		return err
	}

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	data := struct {
		EmbedPath   string
		FrontendDir string
	}{
		EmbedPath:   frontendDir,
		FrontendDir: frontendDir,
	}

	return tmpl.Execute(file, data)
}

func initGoModule(dir string) error {
	log.Println("Initializing Go module...")
	cmd := exec.Command("go", "mod", "init", "gonext")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func buildBinary(tempDir, outputBinary string) error {
	log.Println("Building the final binary...")
	cmd := exec.Command("go", "build", "-o", outputBinary)
	cmd.Dir = tempDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
