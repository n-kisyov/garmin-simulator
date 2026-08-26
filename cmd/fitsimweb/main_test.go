package main

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
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
