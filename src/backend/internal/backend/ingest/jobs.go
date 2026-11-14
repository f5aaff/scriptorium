package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"scriptorium/internal/backend/dao"

	"github.com/google/uuid"
)

var ErrBatchTooLarge = errors.New("import batch exceeds configured limits")

const (
	DefaultJobWorkers       = 2
	MaxBatchFiles           = 1000
	MaxBatchBytes     int64 = 1 << 30
)

type JobRepository interface {
	ActiveIngestionJobCount() (int, error)
	CreateIngestionJob(dao.IngestionJob) error
	GetIngestionJob(string) (dao.IngestionJob, error)
	ListIngestionJobs(int) ([]dao.IngestionJob, error)
	ClaimIngestionJob() (dao.IngestionJob, bool, error)
	UpdateIngestionJob(dao.IngestionJob) error
	RecoverIngestionJobs() error
}

type Upload struct {
	Filename string
	Reader   io.Reader
	Metadata map[string]any
}
type jobItem struct {
	Path     string         `json:"path"`
	Filename string         `json:"filename"`
	Metadata map[string]any `json:"metadata,omitempty"`
}
type ImportRoot struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type JobManager struct {
	repository JobRepository
	ingestor   *Service
	spool      string
	roots      map[string]string
	workers    int
	poll       time.Duration
	mu         sync.Mutex
	enqueueMu  sync.Mutex
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

func NewJobManager(repository JobRepository, ingestor *Service, spool string, roots []string, workers int) (*JobManager, error) {
	if repository == nil || ingestor == nil || strings.TrimSpace(spool) == "" {
		return nil, errors.New("job repository, ingestor and spool directory are required")
	}
	if workers < 1 || workers > 16 {
		workers = DefaultJobWorkers
	}
	abs, err := filepath.Abs(spool)
	if err != nil {
		return nil, err
	}
	parent := filepath.Dir(abs)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return nil, fmt.Errorf("create ingestion spool parent: %w", err)
	}
	parent, err = filepath.EvalSymlinks(parent)
	if err != nil {
		return nil, fmt.Errorf("resolve ingestion spool parent: %w", err)
	}
	abs = filepath.Join(parent, filepath.Base(abs))
	if info, statErr := os.Lstat(abs); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return nil, errors.New("ingestion spool must be a real directory, not a symlink")
		}
	} else if errors.Is(statErr, os.ErrNotExist) {
		if err := os.Mkdir(abs, 0700); err != nil {
			return nil, fmt.Errorf("create ingestion spool: %w", err)
		}
	} else {
		return nil, fmt.Errorf("inspect ingestion spool: %w", statErr)
	}
	if err := os.Chmod(abs, 0700); err != nil {
		return nil, fmt.Errorf("secure ingestion spool: %w", err)
	}
	m := &JobManager{repository: repository, ingestor: ingestor, spool: abs, roots: make(map[string]string), workers: workers, poll: 500 * time.Millisecond}
	for _, root := range roots {
		resolved, err := filepath.EvalSymlinks(root)
		if err != nil {
			return nil, fmt.Errorf("resolve import root %q: %w", root, err)
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("import root %q is not a directory", root)
		}
		m.roots[rootID(resolved)] = filepath.Clean(resolved)
	}
	if err := repository.RecoverIngestionJobs(); err != nil {
		return nil, fmt.Errorf("recover interrupted ingestion jobs: %w", err)
	}
	return m, nil
}

func rootID(root string) string {
	sum := sha256.Sum256([]byte(root))
	return hex.EncodeToString(sum[:8])
}
func (m *JobManager) Roots() []ImportRoot {
	roots := make([]ImportRoot, 0, len(m.roots))
	for id, path := range m.roots {
		roots = append(roots, ImportRoot{ID: id, Name: filepath.Base(path)})
	}
	return roots
}

func (m *JobManager) Start(parent context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	m.cancel = cancel
	for i := 0; i < m.workers; i++ {
		m.wg.Add(1)
		go m.worker(ctx)
	}
}
func (m *JobManager) Stop(ctx context.Context) error {
	m.mu.Lock()
	cancel := m.cancel
	m.cancel = nil
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	done := make(chan struct{})
	go func() { m.wg.Wait(); close(done) }()
	select {
	case <-done:
		return m.repository.RecoverIngestionJobs()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *JobManager) EnqueueUploads(ctx context.Context, uploads []Upload) (dao.IngestionJob, error) {
	m.enqueueMu.Lock()
	defer m.enqueueMu.Unlock()
	if err := m.ensureQueueCapacity(); err != nil {
		return dao.IngestionJob{}, err
	}
	if len(uploads) == 0 || len(uploads) > MaxBatchFiles {
		return dao.IngestionJob{}, fmt.Errorf("batch must contain between 1 and %d files", MaxBatchFiles)
	}
	id := uuid.NewString()
	dir := filepath.Join(m.spool, id)
	if err := os.Mkdir(dir, 0700); err != nil {
		return dao.IngestionJob{}, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(dir)
		}
	}()
	items := make([]jobItem, 0, len(uploads))
	var total int64
	for i, upload := range uploads {
		if err := ctx.Err(); err != nil {
			return dao.IngestionJob{}, err
		}
		if upload.Reader == nil {
			return dao.IngestionJob{}, errors.New("batch contains an empty file stream")
		}
		name := filepath.Base(strings.TrimSpace(upload.Filename))
		if name == "." || name == "" || name == string(filepath.Separator) {
			return dao.IngestionJob{}, errors.New("batch contains an invalid filename")
		}
		path := filepath.Join(dir, fmt.Sprintf("%04d", i))
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return dao.IngestionJob{}, err
		}
		written, copyErr := io.Copy(file, io.LimitReader(contextReader{ctx: ctx, reader: upload.Reader}, DefaultMaxFileSize+1))
		syncErr := file.Sync()
		closeErr := file.Close()
		if copyErr != nil {
			return dao.IngestionJob{}, copyErr
		}
		if syncErr != nil {
			return dao.IngestionJob{}, syncErr
		}
		if closeErr != nil {
			return dao.IngestionJob{}, closeErr
		}
		if written == 0 {
			return dao.IngestionJob{}, fmt.Errorf("%s is empty", name)
		}
		if written > DefaultMaxFileSize {
			return dao.IngestionJob{}, fmt.Errorf("%w: %s exceeds the 100 MiB per-file limit", ErrTooLarge, name)
		}
		total += written
		if total > MaxBatchBytes {
			return dao.IngestionJob{}, fmt.Errorf("%w: 1 GiB total limit", ErrBatchTooLarge)
		}
		items = append(items, jobItem{Path: path, Filename: name, Metadata: upload.Metadata})
	}
	job, err := m.persistJob(id, items)
	if err != nil {
		return dao.IngestionJob{}, err
	}
	cleanup = false
	return job, nil
}

// EnqueueDirectory accepts only an opaque ID for an administrator-configured
// root and a relative path below it. No client-provided absolute path is used.
func (m *JobManager) EnqueueDirectory(ctx context.Context, id, relative string) (dao.IngestionJob, error) {
	m.enqueueMu.Lock()
	defer m.enqueueMu.Unlock()
	if err := m.ensureQueueCapacity(); err != nil {
		return dao.IngestionJob{}, err
	}
	root, ok := m.roots[id]
	if !ok {
		return dao.IngestionJob{}, errors.New("unknown import root")
	}
	if relative == "" {
		relative = "."
	}
	if filepath.IsAbs(relative) || !filepath.IsLocal(relative) {
		return dao.IngestionJob{}, errors.New("import path must be relative to its configured root")
	}
	candidate := filepath.Join(root, filepath.Clean(relative))
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return dao.IngestionJob{}, fmt.Errorf("resolve import path: %w", err)
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return dao.IngestionJob{}, errors.New("import path escapes its configured root")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return dao.IngestionJob{}, err
	}
	if isWithin(m.spool, resolved) {
		return dao.IngestionJob{}, errors.New("cannot import from the private ingestion staging directory")
	}
	idJob := uuid.NewString()
	dir := filepath.Join(m.spool, idJob)
	if err := os.Mkdir(dir, 0700); err != nil {
		return dao.IngestionJob{}, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(dir)
		}
	}()
	items := make([]jobItem, 0)
	var total int64
	addFile := func(path string, info os.FileInfo) error {
		if len(items) >= MaxBatchFiles {
			return fmt.Errorf("directory import exceeds %d files", MaxBatchFiles)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relativePath, err := filepath.Rel(resolved, path)
		if err != nil {
			return err
		}
		source, err := os.Open(path)
		if err != nil {
			return err
		}
		defer source.Close()
		openedInfo, err := source.Stat()
		if err != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
			return errors.New("import source changed while it was being staged")
		}
		destPath := filepath.Join(dir, fmt.Sprintf("%04d", len(items)))
		dest, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		written, copyErr := io.Copy(dest, io.LimitReader(contextReader{ctx: ctx, reader: source}, DefaultMaxFileSize+1))
		syncErr := dest.Sync()
		closeErr := dest.Close()
		if copyErr != nil {
			return copyErr
		}
		if syncErr != nil {
			return syncErr
		}
		if closeErr != nil {
			return closeErr
		}
		if written > DefaultMaxFileSize {
			return fmt.Errorf("%w: %s exceeds the 100 MiB per-file limit", ErrTooLarge, relativePath)
		}
		total += written
		if total > MaxBatchBytes {
			return fmt.Errorf("%w: directory import exceeds the 1 GiB total limit", ErrBatchTooLarge)
		}
		items = append(items, jobItem{Path: destPath, Filename: filepath.Base(path)})
		return nil
	}
	if info.IsDir() {
		err = filepath.WalkDir(resolved, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if path == resolved {
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if entry.IsDir() && isWithin(path, m.spool) {
				return filepath.SkipDir
			}
			if entry.IsDir() {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			return addFile(path, info)
		})
	} else {
		err = addFile(resolved, info)
	}
	if err != nil {
		return dao.IngestionJob{}, fmt.Errorf("stage directory import: %w", err)
	}
	if len(items) == 0 {
		return dao.IngestionJob{}, errors.New("import path contains no regular files")
	}
	job, err := m.persistJob(idJob, items)
	if err != nil {
		return dao.IngestionJob{}, err
	}
	cleanup = false
	return job, nil
}

func (m *JobManager) ensureQueueCapacity() error {
	active, err := m.repository.ActiveIngestionJobCount()
	if err != nil {
		return fmt.Errorf("check ingestion queue capacity: %w", err)
	}
	if active >= dao.MaxActiveIngestionJobs {
		return dao.ErrJobQueueFull
	}
	return nil
}

func (m *JobManager) persistJob(id string, items []jobItem) (dao.IngestionJob, error) {
	payload, err := json.Marshal(items)
	if err != nil {
		return dao.IngestionJob{}, err
	}
	job := dao.IngestionJob{ID: id, Payload: payload, Total: len(items)}
	if err := m.repository.CreateIngestionJob(job); err != nil {
		return dao.IngestionJob{}, fmt.Errorf("queue ingestion job: %w", err)
	}
	return m.repository.GetIngestionJob(id)
}
func (m *JobManager) Get(id string) (dao.IngestionJob, error) {
	return m.repository.GetIngestionJob(id)
}
func (m *JobManager) List(limit int) ([]dao.IngestionJob, error) {
	return m.repository.ListIngestionJobs(limit)
}

func (m *JobManager) worker(ctx context.Context) {
	defer m.wg.Done()
	ticker := time.NewTicker(m.poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		job, ok, err := m.repository.ClaimIngestionJob()
		if err != nil {
			continue
		}
		if !ok {
			continue
		}
		m.process(ctx, job)
	}
}
func (m *JobManager) process(ctx context.Context, job dao.IngestionJob) {
	var items []jobItem
	if err := json.Unmarshal(job.Payload, &items); err != nil || len(items) == 0 || len(items) != job.Total {
		job.Status = "failed"
		job.Error = "invalid persisted job payload"
		job.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		_ = m.repository.UpdateIngestionJob(job)
		return
	}
	job.Processed = 0
	job.Failed = 0
	job.Error = ""
	for _, item := range items {
		if ctx.Err() != nil {
			return
		}
		file, err := os.Open(item.Path)
		if err == nil {
			_, err = m.ingestor.Import(ctx, Request{Filename: item.Filename, Metadata: item.Metadata}, file)
			_ = file.Close()
		}
		// A cancelled import is not a completed failure: leave the job running
		// so shutdown recovery can replay it from the durable staged source.
		if ctx.Err() != nil {
			return
		}
		job.Processed++
		if err != nil {
			job.Failed++
			if job.Error != "" {
				job.Error += "; "
			}
			job.Error += fmt.Sprintf("%s: %v", item.Filename, err)
			if len(job.Error) > 4096 {
				job.Error = job.Error[:4096]
			}
		}
		if job.Processed == job.Total {
			job.Status = "completed"
			if job.Failed > 0 {
				job.Status = "completed_with_errors"
			}
			job.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
		if err := m.repository.UpdateIngestionJob(job); err != nil {
			return
		}
	}
	_ = os.RemoveAll(filepath.Join(m.spool, job.ID))
}

// contextReader lets cancellation interrupt large staging copies rather than
// waiting for an entire file to be copied before workers can shut down.
type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func isWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
