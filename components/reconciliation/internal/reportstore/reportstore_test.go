package reportstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
)

func TestLoadRecent_SortsByReportTimestamp_NotModTime(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := NewFileStore(dir, 0, 0, nil)

	older := &domain.ReconciliationReport{
		RunID:     "run-older",
		Timestamp: time.Date(2025, 12, 30, 20, 0, 0, 0, time.UTC),
	}
	newer := &domain.ReconciliationReport{
		RunID:     "run-newer",
		Timestamp: time.Date(2025, 12, 30, 21, 0, 0, 0, time.UTC),
	}

	if err := store.Save(context.Background(), older); err != nil {
		t.Fatalf("Save(older) failed: %v", err)
	}
	if err := store.Save(context.Background(), newer); err != nil {
		t.Fatalf("Save(newer) failed: %v", err)
	}

	// Force a mismatch where filesystem ModTime would produce the wrong ordering:
	// make the "older" report look newer on disk, and the "newer" report look older.
	now := time.Now()
	olderPath := filepath.Join(dir, store.buildFilename(older))
	newerPath := filepath.Join(dir, store.buildFilename(newer))
	if err := os.Chtimes(olderPath, now.Add(2*time.Hour), now.Add(2*time.Hour)); err != nil {
		t.Fatalf("Chtimes(older) failed: %v", err)
	}
	if err := os.Chtimes(newerPath, now.Add(-2*time.Hour), now.Add(-2*time.Hour)); err != nil {
		t.Fatalf("Chtimes(newer) failed: %v", err)
	}

	reports, err := store.LoadRecent(context.Background(), 2)
	if err != nil {
		t.Fatalf("LoadRecent failed: %v", err)
	}
	if len(reports) != 2 {
		t.Fatalf("expected 2 reports, got %d", len(reports))
	}
	if reports[0].RunID != "run-newer" {
		t.Fatalf("expected newest report first (run-newer), got %q", reports[0].RunID)
	}
	if reports[1].RunID != "run-older" {
		t.Fatalf("expected older report second (run-older), got %q", reports[1].RunID)
	}
}

func TestLoadRecent_FallsBackToFileContentsTimestamp_WhenFilenameNotCanonical(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := NewFileStore(dir, 0, 0, nil)

	// A non-canonical filename that still matches listReportFiles() filtering.
	fromContentsName := "reconciliation_bad.json"
	fromContentsPath := filepath.Join(dir, fromContentsName)
	fromContentsJSON := `{"run_id":"from-contents","timestamp":"2025-12-30T22:00:00Z"}`
	if err := os.WriteFile(fromContentsPath, []byte(fromContentsJSON), 0o600); err != nil {
		t.Fatalf("WriteFile(fromContents) failed: %v", err)
	}
	if err := os.Chtimes(fromContentsPath, time.Now().Add(-24*time.Hour), time.Now().Add(-24*time.Hour)); err != nil {
		t.Fatalf("Chtimes(fromContents) failed: %v", err)
	}

	canonical := &domain.ReconciliationReport{
		RunID:     "canonical",
		Timestamp: time.Date(2025, 12, 30, 21, 0, 0, 0, time.UTC),
	}
	if err := store.Save(context.Background(), canonical); err != nil {
		t.Fatalf("Save(canonical) failed: %v", err)
	}

	reports, err := store.LoadRecent(context.Background(), 2)
	if err != nil {
		t.Fatalf("LoadRecent failed: %v", err)
	}
	if len(reports) != 2 {
		t.Fatalf("expected 2 reports, got %d", len(reports))
	}

	if reports[0].RunID != "from-contents" {
		t.Fatalf("expected report parsed from file contents first, got %q", reports[0].RunID)
	}
	if !reports[0].Timestamp.Equal(time.Date(2025, 12, 30, 22, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected timestamp for from-contents: %s", reports[0].Timestamp.Format(time.RFC3339Nano))
	}
}

func TestLoadRecent_FallsBackToModTime_WhenTimestampMissing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := NewFileStore(dir, 0, 0, nil)

	noTSName := "reconciliation_no_ts.json"
	noTSPath := filepath.Join(dir, noTSName)
	noTSJSON := `{"run_id":"no-ts"}`
	if err := os.WriteFile(noTSPath, []byte(noTSJSON), 0o600); err != nil {
		t.Fatalf("WriteFile(noTS) failed: %v", err)
	}

	// Since parsing fails (timestamp missing), ordering should fall back to ModTime for this file.
	if err := os.Chtimes(noTSPath, time.Now().Add(48*time.Hour), time.Now().Add(48*time.Hour)); err != nil {
		t.Fatalf("Chtimes(noTS) failed: %v", err)
	}

	canonical := &domain.ReconciliationReport{
		RunID:     "canonical",
		Timestamp: time.Date(2025, 12, 30, 21, 0, 0, 0, time.UTC),
	}
	if err := store.Save(context.Background(), canonical); err != nil {
		t.Fatalf("Save(canonical) failed: %v", err)
	}

	reports, err := store.LoadRecent(context.Background(), 2)
	if err != nil {
		t.Fatalf("LoadRecent failed: %v", err)
	}
	if len(reports) != 2 {
		t.Fatalf("expected 2 reports, got %d", len(reports))
	}

	if reports[0].RunID != "no-ts" {
		t.Fatalf("expected modtime-fallback file first, got %q", reports[0].RunID)
	}
}
