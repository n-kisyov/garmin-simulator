package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	http.HandleFunc("/api/simulate", simulateHandler)

	port := "8088"
	fmt.Printf("Starting fitsimweb server on port %s...\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func simulateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form (max 10 MB)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	activity := r.FormValue("activity")
	if activity == "" {
		http.Error(w, "Missing 'activity' field", http.StatusBadRequest)
		return
	}

	// Setup a temporary directory for this request
	tempDir, err := os.MkdirTemp("", "fitsimweb-*")
	if err != nil {
		http.Error(w, "Failed to create temp dir", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tempDir) // cleanup after request

	outFitFile := filepath.Join(tempDir, "output.fit")

	// Construct command arguments
	args := []string{activity}
	
	// Dynamically add all other form fields as flags (except activity and file)
	for key, values := range r.MultipartForm.Value {
		if key != "activity" && key != "file" && len(values) > 0 {
			args = append(args, "--"+key, values[0])
		}
	}

	args = append(args, "--file", outFitFile)

	// Handle uploaded KML file if present
	file, header, err := r.FormFile("kml_file")
	if err == nil {
		defer file.Close()
		kmlPath := filepath.Join(tempDir, header.Filename)
		dst, err := os.Create(kmlPath)
		if err != nil {
			http.Error(w, "Failed to save KML file", http.StatusInternalServerError)
			return
		}
		if _, err := io.Copy(dst, file); err != nil {
			dst.Close()
			http.Error(w, "Failed to write KML file", http.StatusInternalServerError)
			return
		}
		dst.Close()
		args = append(args, "--kml", kmlPath)
	}

	// If datetime is missing, provide a default
	hasDatetime := false
	for _, arg := range args {
		if strings.HasPrefix(arg, "--datetime") {
			hasDatetime = true
			break
		}
	}
	if !hasDatetime {
		args = append(args, "--datetime", time.Now().Format("02-01-06 15:04:05"))
	}

	// Execute fitsim.exe as a subprocess
	// Note: fitsim.exe must be in the same directory or PATH
	cmd := exec.Command("./fitsim.exe", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("fitsim execution failed: %s\nOutput: %s", err, string(output))
		http.Error(w, fmt.Sprintf("Simulation failed: %s\n%s", err, string(output)), http.StatusInternalServerError)
		return
	}

	// Read generated FIT file
	fitData, err := os.ReadFile(outFitFile)
	if err != nil {
		http.Error(w, "Failed to read generated FIT file", http.StatusInternalServerError)
		return
	}

	// Send file as download
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.fit\"", activity))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(fitData)))
	w.Write(fitData)
}
