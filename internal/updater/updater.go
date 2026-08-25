// Package updater downloads and installs one explicit kubePeep release.
//
// It deliberately has no "latest" lookup: checksums and archives are always
// fetched from the same immutable vX.Y.Z release path.
package updater

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	DefaultReleaseBaseURL = "https://github.com/fvmoraes/kubepeep/releases/download"
	maximumChecksumsBytes = 1 << 20
	maximumArchiveBytes   = 256 << 20
	maximumBinaryBytes    = 128 << 20
)

var (
	ErrInvalidVersion = errors.New("update: target version must be an exact X.Y.Z release")
	ErrUnsupported    = errors.New("update: unsupported operating system or architecture")
	ErrChecksum       = errors.New("update: SHA-256 verification failed")
	ErrConcurrent     = errors.New("update: another update is already in progress")
	ErrRollback       = errors.New("update: replacement verification failed and the previous binary was restored")
)

// HTTPDoer is the narrow transport dependency used by Service.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// BinaryVerifier proves that a downloaded executable reports the exact target
// version before it is allowed to replace the running release.
type BinaryVerifier interface {
	Verify(context.Context, string, string) error
}

// BinaryVerifierFunc adapts a function for hermetic tests.
type BinaryVerifierFunc func(context.Context, string, string) error

func (function BinaryVerifierFunc) Verify(ctx context.Context, path, version string) error {
	return function(ctx, path, version)
}

// Options contains composition seams. Production callers should use Default;
// the transport and platform overrides exist for hermetic package tests.
type Options struct {
	Client          HTTPDoer
	Verifier        BinaryVerifier
	ReleaseBaseURL  string
	ExecutablePath  func() (string, error)
	OperatingSystem string
	Architecture    string
	AllowHTTP       bool
}

// Request identifies both ends of an explicit update.
type Request struct {
	CurrentVersion string
	TargetVersion  string
}

// Result describes the verified release and whether Windows scheduled its
// replacement for immediately after this process exits.
type Result struct {
	CurrentVersion string
	TargetVersion  string
	Archive        string
	Scheduled      bool
	AlreadyCurrent bool
}

// Service implements the complete download, verification and replacement
// transaction.
type Service struct {
	client          HTTPDoer
	verifier        BinaryVerifier
	releaseBaseURL  string
	executablePath  func() (string, error)
	operatingSystem string
	architecture    string
	allowHTTP       bool
}

// Default returns the production updater pinned to kubePeep's canonical
// release origin.
func Default() *Service {
	service, err := New(Options{})
	if err != nil {
		panic(err)
	}
	return service
}

// New validates updater composition before any network or filesystem work.
func New(options Options) (*Service, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(options.ReleaseBaseURL), "/")
	if baseURL == "" {
		baseURL = DefaultReleaseBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && !(options.AllowHTTP && parsed.Scheme == "http")) {
		return nil, errors.New("update: release origin must be an absolute HTTPS URL")
	}
	client := options.Client
	if client == nil {
		client = &http.Client{
			Timeout: 45 * time.Second,
			CheckRedirect: func(request *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return errors.New("update: too many download redirects")
				}
				if request.URL.Scheme != "https" {
					return errors.New("update: download redirect must use HTTPS")
				}
				return nil
			},
		}
	}
	verifier := options.Verifier
	if verifier == nil {
		verifier = commandVerifier{}
	}
	executablePath := options.ExecutablePath
	if executablePath == nil {
		executablePath = os.Executable
	}
	operatingSystem := options.OperatingSystem
	if operatingSystem == "" {
		operatingSystem = runtime.GOOS
	}
	architecture := options.Architecture
	if architecture == "" {
		architecture = runtime.GOARCH
	}
	return &Service{
		client: client, verifier: verifier, releaseBaseURL: baseURL,
		executablePath: executablePath, operatingSystem: operatingSystem,
		architecture: architecture, allowHTTP: options.AllowHTTP,
	}, nil
}

// Update performs a bounded download and checksum verification before touching
// the installed executable. No kubePeep data directory is opened or changed.
func (service *Service) Update(ctx context.Context, request Request) (Result, error) {
	targetVersion, err := normalizeVersion(request.TargetVersion)
	if err != nil {
		return Result{}, err
	}
	result := Result{CurrentVersion: request.CurrentVersion, TargetVersion: targetVersion}
	if current, currentErr := normalizeVersion(request.CurrentVersion); currentErr == nil && current == targetVersion {
		result.AlreadyCurrent = true
		return result, nil
	}
	archive, binaryName, err := releaseAsset(targetVersion, service.operatingSystem, service.architecture)
	if err != nil {
		return Result{}, err
	}
	result.Archive = archive

	target, err := service.executablePath()
	if err != nil {
		return Result{}, fmt.Errorf("update: resolve executable: %w", err)
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return Result{}, fmt.Errorf("update: resolve executable: %w", err)
	}
	info, err := os.Lstat(target)
	if err != nil {
		return Result{}, fmt.Errorf("update: inspect executable: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return Result{}, errors.New("update: executable is not a regular file")
	}

	lockPath := filepath.Join(filepath.Dir(target), ".kubePeep.update.lock")
	lock, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return Result{}, ErrConcurrent
	}
	if err != nil {
		return Result{}, fmt.Errorf("update: acquire update lock: %w", err)
	}
	if _, err := fmt.Fprintf(lock, "%d\n", os.Getpid()); err != nil {
		_ = lock.Close()
		_ = os.Remove(lockPath)
		return Result{}, fmt.Errorf("update: initialize update lock: %w", err)
	}
	if err := lock.Sync(); err != nil {
		_ = lock.Close()
		_ = os.Remove(lockPath)
		return Result{}, fmt.Errorf("update: sync update lock: %w", err)
	}
	if err := lock.Close(); err != nil {
		_ = os.Remove(lockPath)
		return Result{}, fmt.Errorf("update: close update lock: %w", err)
	}
	keepLock := false
	defer func() {
		if !keepLock {
			_ = os.Remove(lockPath)
		}
	}()

	temporaryRoot, err := os.MkdirTemp("", "kubepeep-update-")
	if err != nil {
		return Result{}, fmt.Errorf("update: create temporary directory: %w", err)
	}
	defer os.RemoveAll(temporaryRoot)
	if err := os.Chmod(temporaryRoot, 0o700); err != nil {
		return Result{}, fmt.Errorf("update: protect temporary directory: %w", err)
	}

	checksumsURL := service.releaseURL(targetVersion, "checksums.txt")
	checksums, err := service.downloadBytes(ctx, checksumsURL, maximumChecksumsBytes)
	if err != nil {
		return Result{}, err
	}
	expected, err := checksumFor(checksums, archive)
	if err != nil {
		return Result{}, err
	}
	archivePath := filepath.Join(temporaryRoot, archive)
	if err := service.downloadFile(ctx, service.releaseURL(targetVersion, archive), archivePath, maximumArchiveBytes); err != nil {
		return Result{}, err
	}
	actual, err := fileSHA256(archivePath)
	if err != nil {
		return Result{}, fmt.Errorf("update: hash archive: %w", err)
	}
	if subtle.ConstantTimeCompare(expected, actual) != 1 {
		return Result{}, ErrChecksum
	}

	extracted := filepath.Join(temporaryRoot, binaryName)
	if err := extractBinary(archivePath, service.operatingSystem, binaryName, extracted); err != nil {
		return Result{}, err
	}
	staged, err := stageCandidate(extracted, target, info.Mode().Perm())
	if err != nil {
		return Result{}, err
	}
	removeStaged := true
	defer func() {
		if removeStaged {
			_ = os.Remove(staged)
		}
	}()
	if err := service.verifier.Verify(ctx, staged, targetVersion); err != nil {
		return Result{}, fmt.Errorf("update: downloaded binary version check failed: %w", err)
	}
	currentHash, err := fileSHA256(target)
	if err != nil {
		return Result{}, fmt.Errorf("update: hash installed executable: %w", err)
	}
	scheduled, err := installPlatform(ctx, target, staged, targetVersion, hex.EncodeToString(currentHash), lockPath, service.verifier)
	if err != nil {
		return Result{}, err
	}
	result.Scheduled = scheduled
	// On Windows the detached post-exit helper owns and removes the staged
	// candidate. Removing it in this process would race deterministically with
	// the helper, which must wait for this executable to exit before replacing
	// it. Unix replacements consume the staged path synchronously.
	if scheduled {
		removeStaged = false
	}
	keepLock = scheduled
	return result, nil
}

func normalizeVersion(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	value = strings.TrimPrefix(value, "v")
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return "", ErrInvalidVersion
	}
	for _, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return "", ErrInvalidVersion
		}
		for _, character := range part {
			if character < '0' || character > '9' {
				return "", ErrInvalidVersion
			}
		}
	}
	return value, nil
}

func releaseAsset(version, operatingSystem, architecture string) (string, string, error) {
	if operatingSystem != "linux" && operatingSystem != "darwin" && operatingSystem != "windows" {
		return "", "", ErrUnsupported
	}
	if architecture != "amd64" && architecture != "arm64" {
		return "", "", ErrUnsupported
	}
	extension := ".tar.gz"
	binary := "kubePeep"
	if operatingSystem == "windows" {
		extension = ".zip"
		binary += ".exe"
	}
	return fmt.Sprintf("kubePeep_%s_%s_%s%s", version, operatingSystem, architecture, extension), binary, nil
}

func (service *Service) releaseURL(version, asset string) string {
	return fmt.Sprintf("%s/v%s/%s", service.releaseBaseURL, version, asset)
}

func (service *Service) downloadBytes(ctx context.Context, rawURL string, maximum int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("update: create download request: %w", err)
	}
	request.Header.Set("Accept", "application/octet-stream")
	response, err := service.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("update: download release asset: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update: release asset returned HTTP %d", response.StatusCode)
	}
	if response.Request != nil && response.Request.URL != nil && response.Request.URL.Scheme != "https" && !service.allowHTTP {
		return nil, errors.New("update: release asset response did not use HTTPS")
	}
	if response.ContentLength > maximum {
		return nil, errors.New("update: release asset exceeds size limit")
	}
	limited := io.LimitReader(response.Body, maximum+1)
	content, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("update: read release asset: %w", err)
	}
	if int64(len(content)) > maximum {
		return nil, errors.New("update: release asset exceeds size limit")
	}
	return content, nil
}

func (service *Service) downloadFile(ctx context.Context, rawURL, destination string, maximum int64) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("update: create download request: %w", err)
	}
	request.Header.Set("Accept", "application/octet-stream")
	response, err := service.client.Do(request)
	if err != nil {
		return fmt.Errorf("update: download release asset: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("update: release asset returned HTTP %d", response.StatusCode)
	}
	if response.Request != nil && response.Request.URL != nil && response.Request.URL.Scheme != "https" && !service.allowHTTP {
		return errors.New("update: release asset response did not use HTTPS")
	}
	if response.ContentLength > maximum {
		return errors.New("update: release asset exceeds size limit")
	}
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("update: create downloaded asset: %w", err)
	}
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(destination)
		}
	}()
	written, err := io.Copy(file, io.LimitReader(response.Body, maximum+1))
	if err != nil {
		return fmt.Errorf("update: write downloaded asset: %w", err)
	}
	if written > maximum {
		return errors.New("update: release asset exceeds size limit")
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("update: sync downloaded asset: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("update: close downloaded asset: %w", err)
	}
	remove = false
	return nil
}

func checksumFor(content []byte, archive string) ([]byte, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	scanner.Buffer(make([]byte, 4096), maximumChecksumsBytes)
	var match []byte
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != archive {
			continue
		}
		decoded, err := hex.DecodeString(fields[0])
		if err != nil || len(decoded) != sha256.Size {
			return nil, errors.New("update: release checksum entry is invalid")
		}
		if match != nil {
			return nil, errors.New("update: release checksum entry is duplicated")
		}
		match = decoded
	}
	if err := scanner.Err(); err != nil {
		return nil, errors.New("update: release checksum list is invalid")
	}
	if match == nil {
		return nil, errors.New("update: release checksum entry is missing")
	}
	return match, nil
}

func fileSHA256(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return nil, err
	}
	return hash.Sum(nil), nil
}

func extractBinary(archivePath, operatingSystem, binaryName, destination string) error {
	if operatingSystem == "windows" {
		return extractZipBinary(archivePath, binaryName, destination)
	}
	return extractTarBinary(archivePath, binaryName, destination)
}

func extractTarBinary(archivePath, binaryName, destination string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("update: open release archive: %w", err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return errors.New("update: release archive is not valid gzip")
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	found := false
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return errors.New("update: release archive is not valid tar")
		}
		if header.Name != binaryName {
			continue
		}
		if found || header.Typeflag != tar.TypeReg || header.Size < 0 || header.Size > maximumBinaryBytes {
			return errors.New("update: release archive contains an invalid executable entry")
		}
		if err := writeBoundedFile(destination, reader, header.Size); err != nil {
			return err
		}
		found = true
	}
	if !found {
		return errors.New("update: release archive does not contain the expected executable")
	}
	return nil
}

func extractZipBinary(archivePath, binaryName, destination string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return errors.New("update: release archive is not valid zip")
	}
	defer reader.Close()
	found := false
	for _, entry := range reader.File {
		if entry.Name != binaryName {
			continue
		}
		if found || !entry.Mode().IsRegular() || entry.UncompressedSize64 > maximumBinaryBytes {
			return errors.New("update: release archive contains an invalid executable entry")
		}
		input, err := entry.Open()
		if err != nil {
			return errors.New("update: release executable entry cannot be opened")
		}
		err = writeBoundedFile(destination, input, int64(entry.UncompressedSize64))
		closeErr := input.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return errors.New("update: release executable entry cannot be closed")
		}
		found = true
	}
	if !found {
		return errors.New("update: release archive does not contain the expected executable")
	}
	return nil
}

func writeBoundedFile(destination string, source io.Reader, expectedSize int64) error {
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700)
	if err != nil {
		return fmt.Errorf("update: create release executable: %w", err)
	}
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(destination)
		}
	}()
	written, err := io.Copy(file, io.LimitReader(source, maximumBinaryBytes+1))
	if err != nil || written != expectedSize || written > maximumBinaryBytes {
		return errors.New("update: release executable is truncated or exceeds its size limit")
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("update: sync release executable: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("update: close release executable: %w", err)
	}
	remove = false
	return nil
}

func stageCandidate(source, target string, mode os.FileMode) (string, error) {
	input, err := os.Open(source)
	if err != nil {
		return "", fmt.Errorf("update: open release executable: %w", err)
	}
	defer input.Close()
	pattern := ".kubePeep.update.*"
	if strings.EqualFold(filepath.Ext(target), ".exe") {
		pattern += ".exe"
	}
	stage, err := os.CreateTemp(filepath.Dir(target), pattern)
	if err != nil {
		return "", fmt.Errorf("update: create staged executable: %w", err)
	}
	stagePath := stage.Name()
	remove := true
	defer func() {
		_ = stage.Close()
		if remove {
			_ = os.Remove(stagePath)
		}
	}()
	written, err := io.Copy(stage, io.LimitReader(input, maximumBinaryBytes+1))
	if err != nil {
		return "", fmt.Errorf("update: stage release executable: %w", err)
	}
	if written > maximumBinaryBytes {
		return "", fmt.Errorf("update: release executable exceeds size limit")
	}
	// Preserve ordinary read/execute permissions while stripping inherited
	// group/world write bits from a potentially over-permissive installation.
	protectedMode := (mode & 0o755) | 0o500
	if err := stage.Chmod(protectedMode); err != nil {
		return "", fmt.Errorf("update: protect staged executable: %w", err)
	}
	if err := stage.Sync(); err != nil {
		return "", fmt.Errorf("update: sync staged executable: %w", err)
	}
	if err := stage.Close(); err != nil {
		return "", fmt.Errorf("update: close staged executable: %w", err)
	}
	remove = false
	return stagePath, nil
}

type commandVerifier struct{}

func (commandVerifier) Verify(ctx context.Context, path, expectedVersion string) error {
	verifyContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	command := exec.CommandContext(verifyContext, path, "version")
	output, err := command.Output()
	if err != nil {
		return errors.New("candidate did not execute successfully")
	}
	if len(output) > 64<<10 {
		return errors.New("candidate version output exceeded its limit")
	}
	for _, field := range strings.Fields(string(output)) {
		if field == "version="+expectedVersion {
			return nil
		}
	}
	return errors.New("candidate reported a different version")
}
