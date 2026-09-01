package main

import (
	"archive/zip"
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tormoder/fit"
)

// formField is one part of a multipart request body.
type formField struct {
	key      string
	value    string
	filename string // non-empty turns this into a file part
}

func buildRequest(t *testing.T, fields []formField) *http.Request {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for _, f := range fields {
		var (
			w   interface{ Write([]byte) (int, error) }
			err error
		)
		if f.filename != "" {
			w, err = mw.CreateFormFile(f.key, f.filename)
		} else {
			w, err = mw.CreateFormField(f.key)
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(f.value)); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/simulate", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

// isolatedTempRoot points os.MkdirTemp at a nested directory under a root the
// test owns, so that anything escaping the per-request temp dir lands somewhere
// the test can see. Returns the root.
func isolatedTempRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMP", nested)
	t.Setenv("TEMP", nested)
	t.Setenv("TMPDIR", nested)
	return root
}

// chdirToRepoRoot makes ./fitsim.exe resolvable, matching how the server is run.
func chdirToRepoRoot(t *testing.T) {
	t.Helper()
	t.Chdir("../..")
	if _, err := os.Stat("fitsim.exe"); err != nil {
		t.Skip("fitsim.exe not built; run build.ps1 first")
	}
}

func sampleKML(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("pkg/geo/testdata/mymaps_point_to_point.kml")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestUploadFilenameCannotEscapeTempDir pins the upload path. Note that
// mime/multipart already reduces Part.FileName() with filepath.Base (RFC 7578,
// Section 4.2), so joining header.Filename onto the temp dir was never actually
// escapable here; the handler now ignores the client name outright and this test
// keeps both layers honest.
func TestUploadFilenameCannotEscapeTempDir(t *testing.T) {
	chdirToRepoRoot(t)
	root := isolatedTempRoot(t)

	for _, name := range []string{
		`../../../canary.kml`,
		`..\..\..\canary.kml`,
		`../../canary.kml`,
	} {
		t.Run(name, func(t *testing.T) {
			req := buildRequest(t, []formField{
				{key: "activity", value: "run"},
				{key: "speed", value: "10.0"},
				{key: "datetime", value: "26-08-26 09:00:00"},
				{key: "kml_file", value: sampleKML(t), filename: name},
			})
			simulateHandler(httptest.NewRecorder(), req)

			for _, dir := range []string{root, filepath.Join(root, "a"), filepath.Join(root, "a", "b")} {
				if p := filepath.Join(dir, "canary.kml"); fileExists(p) {
					t.Errorf("upload escaped the temp dir and was written to %s", p)
				}
			}
		})
	}
}

// TestClientCannotOverrideOutputPath guards the --file flag, which is chosen by
// the server. The original handler already skipped the "file" key; this pins that
// behaviour now that the filtering has moved to an allowlist.
func TestClientCannotOverrideOutputPath(t *testing.T) {
	chdirToRepoRoot(t)
	root := isolatedTempRoot(t)
	target := filepath.Join(root, "pwned.fit")

	req := buildRequest(t, []formField{
		{key: "activity", value: "run"},
		{key: "speed", value: "10.0"},
		{key: "datetime", value: "26-08-26 09:00:00"},
		{key: "file", value: target},
		{key: "kml_file", value: sampleKML(t), filename: "route.kml"},
	})
	rec := httptest.NewRecorder()
	simulateHandler(rec, req)

	if fileExists(target) {
		t.Errorf("client-supplied --file was honoured; wrote %s", target)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}

// TestClientCannotOverrideKMLPath covers the one genuinely exploitable hole in
// the original handler: with no file uploaded, a "kml" form field was forwarded
// as --kml, so a client could name any KML-parseable path on the server and get
// its coordinates back inside the generated FIT. The request must not succeed,
// because the only route the server will read is one that was actually uploaded.
func TestClientCannotOverrideKMLPath(t *testing.T) {
	chdirToRepoRoot(t)
	isolatedTempRoot(t)

	// A real, parseable route that lives on the server but is never uploaded.
	const serverSidePath = "pkg/geo/testdata/mymaps_point_to_point.kml"

	req := buildRequest(t, []formField{
		{key: "activity", value: "run"},
		{key: "speed", value: "10.0"},
		{key: "datetime", value: "26-08-26 09:00:00"},
		{key: "kml", value: serverSidePath},
	})
	rec := httptest.NewRecorder()
	simulateHandler(rec, req)

	body := rec.Body.Bytes()
	if len(body) > 12 && string(body[8:12]) == ".FIT" {
		t.Errorf("server read the client-named path %s and returned a FIT built from it", serverSidePath)
	}
	if rec.Code == http.StatusOK {
		t.Errorf("status = 200, want a failure: no route was uploaded")
	}
}

func TestActivityIsValidated(t *testing.T) {
	cases := []struct {
		name     string
		activity string
		wantCode int
	}{
		{"traversal", "../../../../evil", http.StatusBadRequest},
		{"header injection", "run\r\nX-Injected: yes", http.StatusBadRequest},
		{"unknown command", "definitely-not-an-activity", http.StatusBadRequest},
		{"empty", "", http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := buildRequest(t, []formField{
				{key: "activity", value: tc.activity},
				{key: "speed", value: "10.0"},
			})
			rec := httptest.NewRecorder()
			simulateHandler(rec, req)

			if rec.Code != tc.wantCode {
				t.Errorf("status = %d, want %d; body: %s", rec.Code, tc.wantCode, rec.Body.String())
			}
			if h := rec.Header().Get("X-Injected"); h != "" {
				t.Errorf("header injection succeeded: X-Injected: %s", h)
			}
		})
	}
}

func TestHappyPathReturnsFIT(t *testing.T) {
	chdirToRepoRoot(t)
	isolatedTempRoot(t)

	req := buildRequest(t, []formField{
		{key: "activity", value: "run"},
		{key: "speed", value: "10.0"},
		{key: "datetime", value: "26-08-26 09:00:00"},
		{key: "kml_file", value: sampleKML(t), filename: "route.kml"},
	})
	rec := httptest.NewRecorder()
	simulateHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.Bytes()
	if len(body) < 12 || string(body[8:12]) != ".FIT" {
		t.Errorf("response is not a FIT file (%d bytes)", len(body))
	}
	if got := rec.Header().Get("Content-Disposition"); got != `attachment; filename="run.fit"` {
		t.Errorf("Content-Disposition = %q", got)
	}
}

// startTimeOf decodes a FIT file from a zip entry and reports when its session
// began, which is the only thing that may differ between files in a series.
func startTimeOf(t *testing.T, f *zip.File) time.Time {
	t.Helper()
	rc, err := f.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()

	decoded, err := fit.Decode(rc)
	if err != nil {
		t.Fatalf("%s: %v", f.Name, err)
	}
	activity, err := decoded.Activity()
	if err != nil {
		t.Fatalf("%s: %v", f.Name, err)
	}
	if len(activity.Sessions) == 0 {
		t.Fatalf("%s: no sessions", f.Name)
	}
	return activity.Sessions[0].StartTime
}

// TestSeriesIsZippedAndOffsetByAMinute covers the point of the count field: the
// response carries every file, they are numbered, and each starts a minute after
// the one before it.
func TestSeriesIsZippedAndOffsetByAMinute(t *testing.T) {
	chdirToRepoRoot(t)
	isolatedTempRoot(t)

	req := buildRequest(t, []formField{
		{key: "activity", value: "run"},
		{key: "speed", value: "10.0"},
		{key: "datetime", value: "26-08-26 09:00:00"},
		{key: "count", value: "3"},
		{key: "kml_file", value: sampleKML(t), filename: "route.kml"},
	})
	rec := httptest.NewRecorder()
	simulateHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Disposition"); got != `attachment; filename="run.zip"` {
		t.Errorf("Content-Disposition = %q", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/zip" {
		t.Errorf("Content-Type = %q, want application/zip", got)
	}

	body := rec.Body.Bytes()
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("response is not a zip: %v", err)
	}

	want := []string{"run1.fit", "run2.fit", "run3.fit"}
	if len(zr.File) != len(want) {
		t.Fatalf("zip holds %d files, want %d", len(zr.File), len(want))
	}
	for i, f := range zr.File {
		if f.Name != want[i] {
			t.Errorf("entry %d = %q, want %q", i, f.Name, want[i])
		}
		gotStart := startTimeOf(t, f)
		wantStart := time.Date(2026, time.August, 26, 9, i, 0, 0, time.UTC)
		if !gotStart.Equal(wantStart) {
			t.Errorf("%s starts at %s, want %s", f.Name, gotStart, wantStart)
		}
	}
}

// TestSingleFileStaysUnzipped pins the default: asking for one file still returns
// a bare .fit, as it did before count existed.
func TestSingleFileStaysUnzipped(t *testing.T) {
	chdirToRepoRoot(t)
	isolatedTempRoot(t)

	req := buildRequest(t, []formField{
		{key: "activity", value: "run"},
		{key: "speed", value: "10.0"},
		{key: "datetime", value: "26-08-26 09:00:00"},
		{key: "count", value: "1"},
		{key: "kml_file", value: sampleKML(t), filename: "route.kml"},
	})
	rec := httptest.NewRecorder()
	simulateHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Disposition"); got != `attachment; filename="run.fit"` {
		t.Errorf("Content-Disposition = %q", got)
	}
	if body := rec.Body.Bytes(); len(body) < 12 || string(body[8:12]) != ".FIT" {
		t.Errorf("response is not a FIT file (%d bytes)", len(body))
	}
}

// TestCountIsValidated keeps the field from turning into a way to make the server
// do unbounded work, and from reaching fitsim as something it cannot parse.
func TestCountIsValidated(t *testing.T) {
	for _, count := range []string{"0", "-1", "101", "abc", "1.5", "1e9", " 2"} {
		t.Run(count, func(t *testing.T) {
			req := buildRequest(t, []formField{
				{key: "activity", value: "run"},
				{key: "speed", value: "10.0"},
				{key: "datetime", value: "26-08-26 09:00:00"},
				{key: "count", value: count},
			})
			rec := httptest.NewRecorder()
			simulateHandler(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// courseKML is a walking loop with elevations baked in, so that a golf request
// through the handler never sends the subprocess off to the elevation API.
func courseKML(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("cmd", "testdata", "course.kml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestGolfRoundGoesThroughTheHandler covers the flags golf adds. The scorecard
// fields have to survive the trip through the allowlist, or the round silently
// comes back as the default eighteen instead of the one that was asked for.
func TestGolfRoundGoesThroughTheHandler(t *testing.T) {
	chdirToRepoRoot(t)
	isolatedTempRoot(t)

	req := buildRequest(t, []formField{
		{key: "activity", value: "golf"},
		{key: "datetime", value: "26-08-26 09:00:00"},
		{key: "holes", value: "9"},
		{key: "par", value: "35"},
		{key: "score", value: "44"},
		{key: "speed", value: "4.5"},
		{key: "kml_file", value: courseKML(t), filename: "course.kml"},
	})
	rec := httptest.NewRecorder()
	simulateHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Disposition"); got != `attachment; filename="golf.fit"` {
		t.Errorf("Content-Disposition = %q", got)
	}

	decoded, err := fit.Decode(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("response does not decode as FIT: %v", err)
	}
	activity, err := decoded.Activity()
	if err != nil {
		t.Fatalf("not an activity file: %v", err)
	}
	if len(activity.Laps) != 9 {
		t.Errorf("got %d laps, want one per hole (9)", len(activity.Laps))
	}
	session := activity.Sessions[0]
	if session.Sport != fit.SportGolf {
		t.Errorf("session.sport = %v, want golf", session.Sport)
	}
	if session.PlayerScore != 44 || session.OpponentScore != 35 {
		t.Errorf("card came back as %d strokes to a par of %d, want 44 to 35",
			session.PlayerScore, session.OpponentScore)
	}
}
