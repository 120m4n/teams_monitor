package dailylog

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRotatesAtMidnight(t *testing.T) {
	dir := t.TempDir()
	w, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 29, 23, 59, 0, 0, time.Local)
	w.now = func() time.Time { return now }
	w.Write([]byte("a\n"))
	w.Write([]byte("b\n"))
	now = now.Add(2 * time.Minute)
	w.Write([]byte("c\n"))
	w.f.Close()

	for name, want := range map[string]string{"teams_monitor-2026-10-29.log": "a\nb\n", "teams_monitor-2026-10-30.log": "c\n"} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(got) != want {
			t.Errorf("%s = %q, %v; want %q", name, got, err, want)
		}
	}
}
