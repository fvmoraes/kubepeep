package kubernetes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// Fingerprint is transient invalidation state. It is never a durable profile
// identity and contains no reversible kubeconfig or credential content.
type Fingerprint [sha256.Size]byte

func (fingerprint Fingerprint) String() string {
	return hex.EncodeToString(fingerprint[:])
}

type trackedFile struct {
	label  string
	path   string
	source bool
}

func referencedFiles(paths []string, raw *clientcmdapi.Config) []trackedFile {
	tracked := make([]trackedFile, 0, len(paths)+len(raw.Clusters)+len(raw.AuthInfos)*3)
	for position, path := range paths {
		tracked = append(tracked, trackedFile{
			label:  fmt.Sprintf("source:%08d", position),
			path:   path,
			source: true,
		})
	}

	var references []trackedFile
	for name, cluster := range raw.Clusters {
		if cluster == nil || cluster.CertificateAuthority == "" || len(cluster.CertificateAuthorityData) != 0 {
			continue
		}
		references = append(references, trackedFile{
			label: "cluster-ca:" + name,
			path:  resolveReference(cluster.LocationOfOrigin, cluster.CertificateAuthority),
		})
	}
	for name, auth := range raw.AuthInfos {
		if auth == nil {
			continue
		}
		if auth.ClientCertificate != "" && len(auth.ClientCertificateData) == 0 {
			references = append(references, trackedFile{
				label: "client-certificate:" + name,
				path:  resolveReference(auth.LocationOfOrigin, auth.ClientCertificate),
			})
		}
		if auth.ClientKey != "" && len(auth.ClientKeyData) == 0 {
			references = append(references, trackedFile{
				label: "client-key:" + name,
				path:  resolveReference(auth.LocationOfOrigin, auth.ClientKey),
			})
		}
		if auth.TokenFile != "" {
			references = append(references, trackedFile{
				label: "token-file:" + name,
				path:  resolveReference(auth.LocationOfOrigin, auth.TokenFile),
			})
		}
	}
	sort.Slice(references, func(left, right int) bool {
		if references[left].label == references[right].label {
			return references[left].path < references[right].path
		}
		return references[left].label < references[right].label
	})
	return append(tracked, references...)
}

func resolveReference(origin, reference string) string {
	if filepath.IsAbs(reference) {
		return filepath.Clean(reference)
	}
	if origin != "" {
		return filepath.Clean(filepath.Join(filepath.Dir(origin), reference))
	}
	absolute, err := filepath.Abs(filepath.Clean(reference))
	if err != nil {
		return filepath.Clean(reference)
	}
	return absolute
}

func fingerprintFiles(ctx context.Context, files []trackedFile, maxFileSize int64) (Fingerprint, error) {
	if ctx == nil {
		return Fingerprint{}, safeError(CodeKubeconfigInvalid, "The kubeconfig could not be fingerprinted safely.", false)
	}
	hasher := sha256.New()
	writeKeyPart(hasher, "kubepeep-kubeconfig-fingerprint-v1")
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return Fingerprint{}, SanitizeError(err)
		}
		writeKeyPart(hasher, file.label)
		writeKeyPart(hasher, file.path)
		if err := hashFile(ctx, hasher, file, maxFileSize); err != nil {
			return Fingerprint{}, err
		}
	}
	var fingerprint Fingerprint
	copy(fingerprint[:], hasher.Sum(nil))
	return fingerprint, nil
}

func hashFile(ctx context.Context, hasher hash.Hash, tracked trackedFile, maxFileSize int64) error {
	before, err := os.Lstat(tracked.path)
	if err != nil {
		if os.IsNotExist(err) && tracked.source {
			return safeError(CodeKubeconfigNotFound, "A selected kubeconfig file is unavailable.", false)
		}
		return safeError(CodeKubeconfigInvalid, "A file referenced by the kubeconfig is unavailable.", false)
	}
	linkTarget := ""
	if before.Mode()&os.ModeSymlink != 0 {
		linkTarget, err = os.Readlink(tracked.path)
		if err != nil {
			return safeError(CodeKubeconfigInvalid, "A kubeconfig file reference changed unexpectedly.", true)
		}
		writeKeyPart(hasher, "symlink")
		writeKeyPart(hasher, linkTarget)
	} else {
		writeKeyPart(hasher, "regular")
	}

	file, err := os.Open(tracked.path)
	if err != nil {
		return safeError(CodeKubeconfigInvalid, "A file referenced by the kubeconfig cannot be read.", false)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() {
		return safeError(CodeKubeconfigInvalid, "A file referenced by the kubeconfig is not a regular file.", false)
	}
	if before.Mode()&os.ModeSymlink == 0 && !os.SameFile(before, opened) {
		return safeError(CodeKubeconfigInvalid, "A kubeconfig file reference changed unexpectedly.", true)
	}
	if opened.Size() > maxFileSize {
		return safeError(CodeKubeconfigInvalid, "A file referenced by the kubeconfig exceeds the size limit.", false)
	}
	writeFileInfo(hasher, opened)
	limited := &contextReader{ctx: ctx, reader: io.LimitReader(file, maxFileSize+1)}
	written, copyErr := io.Copy(hasher, limited)
	if copyErr != nil {
		if ctx.Err() != nil {
			return SanitizeError(ctx.Err())
		}
		return safeError(CodeKubeconfigInvalid, "A file referenced by the kubeconfig cannot be read safely.", true)
	}
	if written > maxFileSize {
		return safeError(CodeKubeconfigInvalid, "A file referenced by the kubeconfig exceeds the size limit.", false)
	}

	after, err := os.Lstat(tracked.path)
	if err != nil || !sameFileSnapshot(before, after) {
		return safeError(CodeKubeconfigInvalid, "A kubeconfig file reference changed unexpectedly.", true)
	}
	if before.Mode()&os.ModeSymlink != 0 {
		afterTarget, readErr := os.Readlink(tracked.path)
		if readErr != nil || afterTarget != linkTarget {
			return safeError(CodeKubeconfigInvalid, "A kubeconfig file reference changed unexpectedly.", true)
		}
	} else if !os.SameFile(opened, after) {
		return safeError(CodeKubeconfigInvalid, "A kubeconfig file reference changed unexpectedly.", true)
	}
	return nil
}

func writeFileInfo(hasher hash.Hash, info os.FileInfo) {
	writeKeyPart(hasher, info.Mode().String())
	writeKeyPart(hasher, fmt.Sprintf("%d", info.Size()))
	writeKeyPart(hasher, fmt.Sprintf("%d", info.ModTime().UnixNano()))
}

func sameFileSnapshot(left, right os.FileInfo) bool {
	return left.Mode() == right.Mode() &&
		left.Size() == right.Size() &&
		left.ModTime().Equal(right.ModTime()) &&
		os.SameFile(left, right)
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *contextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}
