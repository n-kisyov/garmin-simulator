package main

import (
	"archive/zip"
	"bytes"
	"embed"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fitsim/pkg/series"
)

// staticFS holds the browser UI. Embedding it means fitsimweb.exe is still a
// single self-contained binary that can be run from anywhere.
//
//go:embed static
var staticFS embed.FS

func main() {
	http.HandleFunc("/api/simulate", simulateHandler)
	http.Handle("/static/", http.FileServer(http.FS(staticFS)))
	http.HandleFunc("/", indexHandler)

	port := "8088"
	fmt.Printf("Starting fitsimweb server on http://localhost:%s ...\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// indexHandler serves the UI at "/". The catch-all pattern "/" matches every
// path no other pattern claims, so anything else still has to 404 explicitly.
func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	page, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "UI not built into this binary", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(page)
}

// fitsimPath locates the fitsim CLI. Preferring the directory of the running
// binary means the server no longer depends on being launched from the repo
// root, with the old relative path and then PATH kept as fallbacks.
func fitsimPath() string {
	const exe = "fitsim.exe"
	if self, err := os.Executable(); err == nil {
		if p := filepath.Join(filepath.Dir(self), exe); fileExists(p) {
			return p
		}
	}
	if fileExists(exe) {
		return "." + string(os.PathSeparator) + exe
	}
	if p, err := exec.LookPath(exe); err == nil {
		return p
	}
	return "." + string(os.PathSeparator) + exe
}

// maxUploadBytes caps the whole request body, not just the part of it that
// ParseMultipartForm is willing to buffer in memory.
const maxUploadBytes = 10 << 20

// allowedActivities is the set of fitsim subcommands a client may ask for. The
// value also lands in a response header, so it must never be free-form.
var allowedActivities = map[string]bool{
	"cardio": true, "cycle": true, "ebike": true, "field": true, "golf": true,
	"hike": true, "meditation": true, "row": true, "run": true, "ski": true,
	"strength": true, "swim": true, "walk": true, "yoga": true,
}

// allowedFlags is the set of fitsim flags a client may set through form fields.
// "file" and "kml" are deliberately absent: both are filesystem paths that the
// server chooses inside its own temp directory, and letting a client supply them
// would turn this handler into an arbitrary read/write primitive. "count" is
// absent too, but for a different reason: it multiplies the work the subprocess
// does, so parseCount vets it rather than passing it straight through.
var allowedFlags = map[string]bool{
	"datetime": true, "distance": true, "duration": true, "holes": true,
	"par": true, "reps": true, "score": true, "sets": true, "speed": true,
	"sport": true, "type": true,
}

// maxSeriesFiles caps how many FIT files one request may ask for. Each one is a
// full simulation, so an unbounded count would let a single request occupy the
// server for as long as it liked.
const maxSeriesFiles = 100

// parseCount reads the optional "count" form field. Absent or empty means one
// file, which is what every client asked for before the field existed.
func parseCount(raw string) (int, error) {
	if raw == "" {
		return 1, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("'count' must be a whole number")
	}
	if n < 1 || n > maxSeriesFiles {
		return 0, fmt.Errorf("'count' must be between 1 and %d", maxSeriesFiles)
	}
	return n, nil
}

func simulateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)

	// Parse multipart form (max 10 MB)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	activity := r.FormValue("activity")
	if activity == "" {
		http.Error(w, "Missing 'activity' field", http.StatusBadRequest)
		return
	}
	if !allowedActivities[activity] {
		http.Error(w, "Unknown 'activity' value", http.StatusBadRequest)
		return
	}

	count, err := parseCount(r.FormValue("count"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Setup a temporary directory for this request
	tempDir, err := os.MkdirTemp("", "fitsimweb-*")
	if err != nil {
		http.Error(w, "Failed to create temp dir", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tempDir) // cleanup after request

	// The base name decides what the series is called: with --count 3, fitsim
	// turns "run.fit" into run1.fit, run2.fit and run3.fit, and those are the
	// names the client sees inside the zip.
	outFitFile := filepath.Join(tempDir, activity+".fit")

	// Construct command arguments
	args := []string{activity}
	
	// Add the recognised form fields as flags. Values are passed in "--flag=value"
	// form so that a value beginning with a dash can never be re-read as a flag
	// of its own.
	for key, values := range r.MultipartForm.Value {
		if allowedFlags[key] && len(values) > 0 {
			args = append(args, "--"+key+"="+values[0])
		}
	}

	args = append(args, "--file", outFitFile, "--count", strconv.Itoa(count))

	// Handle uploaded KML file if present. The client-supplied filename is
	// discarded rather than joined onto tempDir. mime/multipart already reduces
	// it with filepath.Base (RFC 7578, Section 4.2), so this is belt and braces
	// rather than a live traversal hole, but the server has no reason to let a
	// client name a file on its disk at all.
	file, _, err := r.FormFile("kml_file")
	if err == nil {
		defer file.Close()
		kmlPath := filepath.Join(tempDir, "route.kml")
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
	cmd := exec.Command(fitsimPath(), args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("fitsim execution failed: %s\nOutput: %s", err, string(output))
		http.Error(w, fmt.Sprintf("Simulation failed: %s\n%s", err, string(output)), http.StatusInternalServerError)
		return
	}

	// One file goes back as-is; a series is zipped, because a single response
	// body cannot carry several downloads.
	body, filename, contentType, err := collectOutput(tempDir, activity, count)
	if err != nil {
		log.Printf("collecting output failed: %s", err)
		http.Error(w, "Failed to read generated FIT file", http.StatusInternalServerError)
		return
	}

	// Send file as download
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
	w.Write(body)
}

// collectOutput gathers what fitsim wrote into dir and returns the response
// body along with the name and type to serve it under. The series is small
// enough (maxSeriesFiles activities) to hold in memory, and buffering it means
// a read failure is still a clean 500 rather than a truncated download.
func collectOutput(dir, activity string, count int) ([]byte, string, string, error) {
	if count == 1 {
		data, err := os.ReadFile(filepath.Join(dir, activity+".fit"))
		return data, activity + ".fit", "application/octet-stream", err
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for i := 1; i <= count; i++ {
		name := series.Filename(activity+".fit", i, count)
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, "", "", err
		}
		entry, err := zw.Create(name)
		if err != nil {
			return nil, "", "", err
		}
		if _, err := entry.Write(data); err != nil {
			return nil, "", "", err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, "", "", err
	}
	return buf.Bytes(), activity + ".zip", "application/zip", nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
