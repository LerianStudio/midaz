// Package reportstore provides persistence for reconciliation reports.
package reportstore

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
)

// Static errors for report store operations.
var (
	ErrReportNil = errors.New("report is nil")
)

const (
	reportFilenameTimestampLayout = "20060102_150405"
	// dirPermissions is the permission mode for the report directory.
	dirPermissions = 0o750
)

// Logger provides minimal logging for the report store.
type Logger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

// Store defines report persistence operations.
type Store interface {
	Save(ctx context.Context, report *domain.ReconciliationReport) error
	LoadLatest(ctx context.Context) (*domain.ReconciliationReport, error)
	LoadRecent(ctx context.Context, limit int) ([]*domain.ReconciliationReport, error)
}

// FileStore persists reports on disk as JSON files.
type FileStore struct {
	dir           string
	maxFiles      int
	retentionDays int
	logger        Logger
	mu            sync.Mutex
}

// NewFileStore creates a new file store.
func NewFileStore(dir string, maxFiles, retentionDays int, logger Logger) *FileStore {
	return &FileStore{
		dir:           dir,
		maxFiles:      maxFiles,
		retentionDays: retentionDays,
		logger:        logger,
	}
}

// Save persists a report to disk.
func (s *FileStore) Save(ctx context.Context, report *domain.ReconciliationReport) error {
	if report == nil {
		return ErrReportNil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, dirPermissions); err != nil {
		return err
	}

	filename := s.buildFilename(report)

	tmpFile, err := os.CreateTemp(s.dir, "reconciliation_*.tmp")
	if err != nil {
		return err
	}

	encoder := json.NewEncoder(tmpFile)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(report); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())

		return err
	}

	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpFile.Name())
		return err
	}

	finalPath := filepath.Join(s.dir, filename)
	if err := os.Rename(tmpFile.Name(), finalPath); err != nil {
		os.Remove(tmpFile.Name())
		return err
	}

	s.prune()

	return nil
}

// LoadLatest loads the most recent report from disk.
func (s *FileStore) LoadLatest(ctx context.Context) (*domain.ReconciliationReport, error) {
	reports, err := s.LoadRecent(ctx, 1)
	if err != nil {
		return nil, err
	}

	if len(reports) == 0 {
		return nil, nil
	}

	return reports[0], nil
}

// LoadRecent loads the N most recent reports.
func (s *FileStore) LoadRecent(ctx context.Context, limit int) ([]*domain.ReconciliationReport, error) {
	if limit <= 0 {
		return []*domain.ReconciliationReport{}, nil
	}

	files, err := s.listReportFiles()
	if err != nil {
		return nil, err
	}

	s.sortReportFilesNewestFirst(files)

	if len(files) > limit {
		files = files[:limit]
	}

	reports := make([]*domain.ReconciliationReport, 0, len(files))
	for _, file := range files {
		_ = ctx // reserved for future cancellation-aware I/O
		path := filepath.Join(s.dir, file.Name())

		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var report domain.ReconciliationReport
		if err := json.Unmarshal(data, &report); err != nil {
			continue
		}

		reports = append(reports, &report)
	}

	return reports, nil
}

func (s *FileStore) buildFilename(report *domain.ReconciliationReport) string {
	ts := report.Timestamp.UTC().Format(reportFilenameTimestampLayout)

	runID := report.RunID
	if runID == "" {
		runID = "unknown"
	}

	return "reconciliation_" + ts + "_" + runID + ".json"
}

func (s *FileStore) sortReportFilesNewestFirst(files []fs.FileInfo) {
	effectiveTime := s.newReportEffectiveTimeFunc(len(files))
	s.sortReportFilesNewestFirstWith(files, effectiveTime)
}

func (s *FileStore) sortReportFilesNewestFirstWith(files []fs.FileInfo, effectiveTime func(fi fs.FileInfo) time.Time) {
	sort.Slice(files, func(i, j int) bool {
		ti := effectiveTime(files[i])

		tj := effectiveTime(files[j])
		if ti.Equal(tj) {
			// Deterministic tie-breaker (newer-ish filenames tend to sort later lexicographically).
			return files[i].Name() > files[j].Name()
		}

		return ti.After(tj)
	})
}

func (s *FileStore) newReportEffectiveTimeFunc(sizeHint int) func(fi fs.FileInfo) time.Time {
	type tsResult struct {
		ts time.Time
		ok bool
	}

	if sizeHint < 0 {
		sizeHint = 0
	}

	cache := make(map[string]tsResult, sizeHint)

	return func(fi fs.FileInfo) time.Time {
		name := fi.Name()
		if res, ok := cache[name]; ok {
			if res.ok {
				return res.ts
			}

			return fi.ModTime()
		}

		var res tsResult
		if parsedTS, ok := parseReportTimestampFromFilename(name); ok {
			res = tsResult{ts: parsedTS, ok: true}
		} else if fileTS, ok := s.parseReportTimestampFromFile(filepath.Join(s.dir, name)); ok {
			res = tsResult{ts: fileTS, ok: true}
		} else {
			res = tsResult{ok: false}
		}

		cache[name] = res
		if res.ok {
			return res.ts
		}

		return fi.ModTime()
	}
}

func parseReportTimestampFromFilename(name string) (time.Time, bool) {
	// Expected canonical format (from buildFilename):
	// reconciliation_YYYYMMDD_HHMMSS_<runid>.json
	if !strings.HasPrefix(name, "reconciliation_") || !strings.HasSuffix(name, ".json") {
		return time.Time{}, false
	}

	base := strings.TrimSuffix(name, ".json")
	rest := strings.TrimPrefix(base, "reconciliation_")

	if len(rest) <= len(reportFilenameTimestampLayout) || rest[len(reportFilenameTimestampLayout)] != '_' {
		return time.Time{}, false
	}

	tsPart := rest[:len(reportFilenameTimestampLayout)]

	ts, err := time.Parse(reportFilenameTimestampLayout, tsPart)
	if err != nil {
		return time.Time{}, false
	}

	if ts.IsZero() {
		return time.Time{}, false
	}

	return ts, true
}

func (s *FileStore) parseReportTimestampFromFile(path string) (time.Time, bool) {
	// Parse minimal JSON to avoid coupling to the full report schema.
	type timestampOnly struct {
		Timestamp time.Time `json:"timestamp"`
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}, false
	}

	var t timestampOnly
	if err := json.Unmarshal(data, &t); err != nil {
		return time.Time{}, false
	}

	if t.Timestamp.IsZero() {
		return time.Time{}, false
	}

	return t.Timestamp, true
}

func (s *FileStore) listReportFiles() ([]fs.FileInfo, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []fs.FileInfo{}, nil
		}

		return nil, err
	}

	files := make([]fs.FileInfo, 0, len(entries))

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if !strings.HasPrefix(entry.Name(), "reconciliation_") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		files = append(files, info)
	}

	return files, nil
}

func (s *FileStore) prune() {
	files, err := s.listReportFiles()
	if err != nil {
		if s.logger != nil {
			s.logger.Errorf("reportstore: failed to list report files for pruning: %v", err)
		}

		return
	}

	effectiveTime := s.newReportEffectiveTimeFunc(len(files))
	s.sortReportFilesNewestFirstWith(files, effectiveTime)

	// Remove by retention days
	removedByRetention := s.pruneByRetention(files, effectiveTime)

	// Refresh file list after retention pruning
	files = s.refreshFilesAfterRetention(files, removedByRetention, effectiveTime)

	// Remove by maxFiles
	s.pruneByMaxFiles(files)
}

// pruneByRetention removes files older than retentionDays and returns a set of removed file names.
func (s *FileStore) pruneByRetention(files []fs.FileInfo, effectiveTime func(fs.FileInfo) time.Time) map[string]struct{} {
	removed := map[string]struct{}{}

	if s.retentionDays <= 0 {
		return removed
	}

	cutoff := time.Now().Add(-time.Duration(s.retentionDays) * 24 * time.Hour)

	for _, file := range files {
		if !effectiveTime(file).Before(cutoff) {
			continue
		}

		path := filepath.Join(s.dir, file.Name())

		if err := os.Remove(path); err != nil {
			s.logWarnIfNotMissing("reportstore: failed to remove report file (retention): %s: %v", path, err)
			continue
		}

		removed[file.Name()] = struct{}{}
	}

	return removed
}

// refreshFilesAfterRetention refreshes the file list after retention pruning.
func (s *FileStore) refreshFilesAfterRetention(files []fs.FileInfo, removedByRetention map[string]struct{}, effectiveTime func(fs.FileInfo) time.Time) []fs.FileInfo {
	if len(removedByRetention) == 0 {
		return files
	}

	refreshed, err := s.listReportFiles()
	if err == nil {
		s.sortReportFilesNewestFirstWith(refreshed, effectiveTime)
		return refreshed
	}

	if s.logger != nil {
		s.logger.Errorf("reportstore: failed to re-list report files after retention pruning: %v", err)
	}

	// Fall back to filtering the original slice to avoid miscounting already-deleted entries.
	filtered := make([]fs.FileInfo, 0, len(files))

	for _, f := range files {
		if _, removed := removedByRetention[f.Name()]; !removed {
			filtered = append(filtered, f)
		}
	}

	return filtered
}

// pruneByMaxFiles removes excess files beyond maxFiles limit.
func (s *FileStore) pruneByMaxFiles(files []fs.FileInfo) {
	if s.maxFiles <= 0 || len(files) <= s.maxFiles {
		return
	}

	for _, file := range files[s.maxFiles:] {
		path := filepath.Join(s.dir, file.Name())

		if err := os.Remove(path); err != nil {
			s.logWarnIfNotMissing("reportstore: failed to remove report file (maxFiles): %s: %v", path, err)
		}
	}
}

// logWarnIfNotMissing logs a warning unless the error is os.IsNotExist.
func (s *FileStore) logWarnIfNotMissing(format string, path string, err error) {
	if s.logger != nil && !os.IsNotExist(err) {
		s.logger.Warnf(format, path, err)
	}
}
