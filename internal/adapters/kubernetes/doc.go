// Package kubernetes contains the credential-safe boundary between kubePeep
// and client-go. Kubeconfig bytes and rest.Config values never leave this
// adapter as serializable domain data.
package kubernetes
