// Package dailylog is an io.Writer that writes to <dir>/teams_monitor-YYYY-MM-DD.log,
// switching to a new file when the local date changes.
package dailylog

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Writer struct {
	dir string
	now func() time.Time

	mu  sync.Mutex
	day string
	f   *os.File
}

// New creates dir if needed. Files are never deleted.
// ponytail: no retention, add a max-age sweep if the folder grows too big.
func New(dir string) (*Writer, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Writer{dir: dir, now: time.Now}, nil
}

func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if day := w.now().Format("2006-01-02"); day != w.day {
		f, err := os.OpenFile(filepath.Join(w.dir, "teams_monitor-"+day+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return 0, err
		}
		if w.f != nil {
			w.f.Close()
		}
		w.f, w.day = f, day
	}
	return w.f.Write(p)
}
