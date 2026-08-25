// Package resources contains the read-only Kubernetes resource domain.
//
// The package deliberately owns public DTOs and ports instead of exposing
// client-go objects to HTTP handlers. Secret access is represented by a
// separate metadata-only port so an adapter cannot accidentally return Secret
// values through a generic resource path.
package resources
