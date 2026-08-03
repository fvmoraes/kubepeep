package logging

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	gingerlogger "github.com/fvmoraes/ginger/pkg/logger"
	"github.com/fvmoraes/kubepeep/internal/securefs"
)

const (
	DefaultMaxBytes   int64 = 10 << 20
	DefaultMaxBackups       = 5
	DefaultMaxAge           = 14 * 24 * time.Hour
)

type Options struct {
	Level        slog.Leveler
	MaxBytes     int64
	MaxBackups   int
	MaxAge       time.Duration
	Now          func() time.Time
	OnDegraded   func(error)
	RetryBackoff time.Duration
}

type Sink struct {
	mu           sync.Mutex
	path         string
	guard        *securefs.Guard
	file         *os.File
	size         int64
	maxBytes     int64
	maxBackups   int
	maxAge       time.Duration
	now          func() time.Time
	onDegraded   func(error)
	retryBackoff time.Duration
	nextRetry    time.Time
	healthy      bool
	closed       bool
}

func New(path string, stdout io.Writer, options Options) (*gingerlogger.Logger, *Sink, error) {
	if stdout == nil {
		stdout = os.Stdout
	}
	sink, err := newSink(path, options)
	if err != nil {
		return nil, nil, err
	}
	handler := newJSONHandler(io.MultiWriter(stdout, sink), options.Level)
	return &gingerlogger.Logger{Logger: slog.New(handler)}, sink, nil
}

func newSink(path string, options Options) (*Sink, error) {
	if path == "" {
		return nil, errors.New("log path is required")
	}
	if options.MaxBytes <= 0 {
		options.MaxBytes = DefaultMaxBytes
	}
	if options.MaxBackups <= 0 {
		options.MaxBackups = DefaultMaxBackups
	}
	if options.MaxAge <= 0 {
		options.MaxAge = DefaultMaxAge
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.RetryBackoff <= 0 {
		options.RetryBackoff = time.Second
	}

	sink := &Sink{
		path:         path,
		maxBytes:     options.MaxBytes,
		maxBackups:   options.MaxBackups,
		maxAge:       options.MaxAge,
		now:          options.Now,
		onDegraded:   options.OnDegraded,
		retryBackoff: options.RetryBackoff,
	}
	if err := securefs.ValidatePrivateDirectory(filepath.Dir(path)); err != nil {
		return nil, fmt.Errorf("validate local log directory: %w", err)
	}
	if err := sink.open(); err != nil {
		return nil, fmt.Errorf("open local log sink: %w", err)
	}
	if err := sink.cleanupBackups(); err != nil {
		_ = sink.guard.Close()
		return nil, fmt.Errorf("clean local log backups: %w", err)
	}
	sink.healthy = true
	return sink, nil
}

func (s *Sink) Write(payload []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, errors.New("log sink is closed")
	}
	if s.file == nil {
		if s.now().Before(s.nextRetry) {
			return len(payload), nil
		}
		if err := s.open(); err != nil {
			s.degrade(err)
			return len(payload), nil
		}
	}
	if err := s.guard.Validate(); err != nil {
		s.degrade(err)
		return len(payload), nil
	}
	if s.size > 0 && s.size+int64(len(payload)) > s.maxBytes {
		if err := s.rotate(); err != nil {
			s.degrade(err)
			return len(payload), nil
		}
	}
	n, err := s.file.Write(payload)
	if err != nil {
		s.degrade(err)
		return len(payload), nil
	}
	s.size += int64(n)
	s.healthy = true
	return n, nil
}

func (s *Sink) Healthy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.healthy && !s.closed
}

func (s *Sink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	s.healthy = false
	if s.file == nil {
		return nil
	}
	if err := s.file.Sync(); err != nil {
		_ = s.file.Close()
		return err
	}
	err := s.guard.Close()
	s.guard = nil
	s.file = nil
	return err
}

func (s *Sink) open() error {
	guard, err := openGuardedLog(s.path)
	if err != nil {
		return err
	}
	file := guard.File()
	info, err := file.Stat()
	if err != nil {
		_ = guard.Close()
		return err
	}
	s.file = file
	s.guard = guard
	s.size = info.Size()
	s.healthy = true
	return nil
}

func openGuardedLog(path string) (*securefs.Guard, error) {
	guard, err := securefs.OpenRegular(path, os.O_APPEND|os.O_WRONLY)
	if errors.Is(err, os.ErrNotExist) {
		guard, err = securefs.CreateExclusive(path)
		if errors.Is(err, os.ErrExist) {
			guard, err = securefs.OpenRegular(path, os.O_APPEND|os.O_WRONLY)
		}
	}
	if err != nil {
		return nil, err
	}
	if err := guard.Protect(0o600); err != nil {
		_ = guard.Close()
		return nil, err
	}
	if _, err := guard.File().Seek(0, io.SeekEnd); err != nil {
		_ = guard.Close()
		return nil, err
	}
	return guard, nil
}

func (s *Sink) rotate() error {
	if s.file == nil || s.guard == nil {
		return errors.New("log file is not open")
	}
	if err := s.guard.Validate(); err != nil {
		return err
	}
	if err := s.file.Sync(); err != nil {
		return err
	}

	backup, err := s.nextBackupName()
	if err != nil {
		return err
	}
	rotated := s.guard
	if err := rotated.PublishNoReplace(backup); err != nil {
		return err
	}
	s.guard = nil
	s.file = nil
	if err := s.open(); err != nil {
		restoreErr := rotated.PublishNoReplace(s.path)
		if restoreErr == nil {
			s.guard = rotated
			s.file = rotated.File()
			if info, statErr := s.file.Stat(); statErr == nil {
				s.size = info.Size()
				_, _ = s.file.Seek(0, io.SeekEnd)
			}
		} else {
			_ = rotated.Close()
		}
		return errors.Join(err, restoreErr)
	}
	if err := rotated.Close(); err != nil {
		return err
	}
	return s.cleanupBackups()
}

func (s *Sink) nextBackupName() (string, error) {
	stamp := s.now().UTC().Format("20060102T150405.000000000Z")
	for sequence := 1; sequence <= 999; sequence++ {
		candidate := fmt.Sprintf("%s.%s.%03d", s.path, stamp, sequence)
		if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", errors.New("log backup sequence exhausted")
}

func (s *Sink) cleanupBackups() error {
	directory := filepath.Dir(s.path)
	prefix := filepath.Base(s.path) + "."
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	type backup struct {
		path    string
		modTime time.Time
	}
	var backups []backup
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), prefix) || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		backups = append(backups, backup{path: filepath.Join(directory, entry.Name()), modTime: info.ModTime()})
	}
	sort.Slice(backups, func(i, j int) bool {
		if backups[i].modTime.Equal(backups[j].modTime) {
			return backups[i].path > backups[j].path
		}
		return backups[i].modTime.After(backups[j].modTime)
	})
	now := s.now()
	kept := 0
	for _, candidate := range backups {
		if now.Sub(candidate.modTime) > s.maxAge || kept >= s.maxBackups {
			if err := os.Remove(candidate.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			continue
		}
		kept++
	}
	return nil
}

func (s *Sink) degrade(err error) {
	if s.guard != nil {
		_ = s.guard.Close()
	}
	s.guard = nil
	s.file = nil
	s.healthy = false
	s.nextRetry = s.now().Add(s.retryBackoff)
	if s.onDegraded != nil {
		s.onDegraded(fmt.Errorf("local log sink degraded: %w", err))
	}
}
