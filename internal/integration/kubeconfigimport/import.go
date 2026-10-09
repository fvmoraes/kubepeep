// Package kubeconfigimport manages explicitly requested local kubeconfigs.
package kubeconfigimport

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"k8s.io/client-go/tools/clientcmd"
	clientapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/fvmoraes/kubepeep/internal/services/clusterprofiles"
)

const maxBytes = 1 << 20

type Importer struct {
	Home string
	mu   sync.Mutex
}

func (i *Importer) Import(ctx context.Context, request clusterprofiles.ImportRequest) (string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if (request.Path == "") == (request.Content == "") {
		return "", clusterprofiles.ErrImportInvalid
	}
	if request.Path != "" {
		path := request.Path
		if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
			path = filepath.Join(i.Home, path[2:])
		}
		if !filepath.IsAbs(path) {
			return "", clusterprofiles.ErrImportInvalid
		}
		path = filepath.Clean(path)
		data, err := readConfig(path)
		if err != nil {
			return "", fmt.Errorf("read kubeconfig source: %w", err)
		}
		if _, err := parse(data); err != nil {
			return "", err
		}
		return path, nil
	}
	config, err := parse([]byte(request.Content))
	if err != nil {
		return "", err
	}
	// A browser file picker cannot supply the original directory. Moving a
	// relative credential reference would silently change which file is read.
	for _, cluster := range config.Clusters {
		if relative(cluster.CertificateAuthority) {
			return "", clusterprofiles.ErrImportRelativePaths
		}
	}
	for _, user := range config.AuthInfos {
		if relative(user.ClientCertificate) || relative(user.ClientKey) || relative(user.TokenFile) || (user.Exec != nil && strings.ContainsAny(user.Exec.Command, `/\`) && relative(user.Exec.Command)) {
			return "", clusterprofiles.ErrImportRelativePaths
		}
	}
	if !filepath.IsAbs(i.Home) {
		return "", clusterprofiles.ErrImportUnavailable
	}
	dir := filepath.Join(i.Home, ".kube")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create kubeconfig directory: %w", err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return "", fmt.Errorf("inspect kubeconfig directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", clusterprofiles.ErrImportUnavailable
	}
	path := filepath.Join(dir, "config")
	previous, err := readConfig(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read existing kubeconfig: %w", err)
	}
	existed := err == nil
	if existed {
		existing, err := parse(previous)
		if err != nil {
			return "", clusterprofiles.ErrImportConflict
		}
		if err := merge(existing.Clusters, config.Clusters); err != nil {
			return "", err
		}
		if err := merge(existing.AuthInfos, config.AuthInfos); err != nil {
			return "", err
		}
		if err := merge(existing.Contexts, config.Contexts); err != nil {
			return "", err
		}
		if err := merge(existing.Extensions, config.Extensions); err != nil {
			return "", err
		}
		if existing.CurrentContext == "" {
			existing.CurrentContext = config.CurrentContext
		}
		config = existing
	}
	encoded, err := clientcmd.Write(*config)
	if err != nil {
		return "", clusterprofiles.ErrImportInvalid
	}
	if len(encoded) > maxBytes {
		return "", clusterprofiles.ErrImportInvalid
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := atomicWrite(path, encoded, previous, existed); err != nil {
		return "", err
	}
	return path, nil
}

func relative(path string) bool { return path != "" && !filepath.IsAbs(path) }

func parse(data []byte) (*clientapi.Config, error) {
	if len(data) == 0 || len(data) > maxBytes {
		return nil, clusterprofiles.ErrImportInvalid
	}
	config, err := clientcmd.Load(data)
	// Parser/validator errors can contain credential-bearing input. Do not
	// propagate or log them. Validation does not contact the cluster or exec.
	if err != nil || len(config.Contexts) == 0 {
		return nil, clusterprofiles.ErrImportInvalid
	}
	// Validate structure without opening credential files. Their paths resolve
	// relative to the source at activation, not this process's working directory.
	validation := config.DeepCopy()
	for _, cluster := range validation.Clusters {
		if cluster == nil {
			return nil, clusterprofiles.ErrImportInvalid
		}
		if cluster.CertificateAuthority != "" {
			if len(cluster.CertificateAuthorityData) != 0 {
				return nil, clusterprofiles.ErrImportInvalid
			}
			cluster.CertificateAuthority = ""
		}
	}
	for _, user := range validation.AuthInfos {
		if user == nil {
			return nil, clusterprofiles.ErrImportInvalid
		}
		if user.ClientCertificate != "" {
			if len(user.ClientCertificateData) != 0 {
				return nil, clusterprofiles.ErrImportInvalid
			}
			user.ClientCertificate, user.ClientCertificateData = "", []byte("reference")
		}
		if user.ClientKey != "" {
			if len(user.ClientKeyData) != 0 {
				return nil, clusterprofiles.ErrImportInvalid
			}
			user.ClientKey, user.ClientKeyData = "", []byte("reference")
		}
	}
	for _, entry := range validation.Contexts {
		if entry == nil {
			return nil, clusterprofiles.ErrImportInvalid
		}
	}
	if err := clientcmd.Validate(*validation); err != nil {
		return nil, clusterprofiles.ErrImportInvalid
	}
	return config, nil
}

func readConfig(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxBytes {
		return nil, clusterprofiles.ErrImportInvalid
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxBytes+1))
	closeErr := file.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return nil, err
	}
	if len(data) > maxBytes {
		return nil, clusterprofiles.ErrImportInvalid
	}
	return data, nil
}

func merge[T any](existing, incoming map[string]T) error {
	for name, value := range incoming {
		if old, exists := existing[name]; exists && !reflect.DeepEqual(old, value) {
			return clusterprofiles.ErrImportConflict
		}
		existing[name] = value
	}
	return nil
}

func atomicWrite(path string, data, previous []byte, existed bool) (result error) {
	file, err := os.CreateTemp(filepath.Dir(path), ".kubepeep-import-*")
	if err != nil {
		return fmt.Errorf("create kubeconfig temporary file: %w", err)
	}
	temporary := file.Name()
	defer func() {
		if err := os.Remove(temporary); err != nil && !errors.Is(err, os.ErrNotExist) {
			result = errors.Join(result, fmt.Errorf("remove kubeconfig temporary file: %w", err))
		}
	}()
	// CreateTemp is private (0600); never use the uploaded file's mode.
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return fmt.Errorf("write kubeconfig: %w", err)
	}
	latest, readErr := readConfig(path)
	if existed && (readErr != nil || !bytes.Equal(previous, latest)) || !existed && !errors.Is(readErr, os.ErrNotExist) {
		return clusterprofiles.ErrImportConflict
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("replace kubeconfig: %w", err)
	}
	return nil
}
