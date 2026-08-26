package series

import "testing"

func TestFilename(t *testing.T) {
	cases := []struct {
		name  string
		file  string
		idx   int
		count int
		want  string
	}{
		{"single file keeps its name", "run.fit", 1, 1, "run.fit"},
		{"first of a series", "run.fit", 1, 3, "run1.fit"},
		{"last of a series", "run.fit", 3, 3, "run3.fit"},
		{"double digits", "run.fit", 10, 12, "run10.fit"},
		{"number goes before the extension", "out/my run.FIT", 2, 2, "out/my run2.FIT"},
		{"no extension", "run", 2, 2, "run2"},
		{"dotted directory", "a.b/run.fit", 2, 2, "a.b/run2.fit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Filename(tc.file, tc.idx, tc.count); got != tc.want {
				t.Errorf("Filename(%q, %d, %d) = %q, want %q", tc.file, tc.idx, tc.count, got, tc.want)
			}
		})
	}
}
