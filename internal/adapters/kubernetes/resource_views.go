package kubernetes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

const maximumViewResponseBytes = 16 << 20

var ErrTableUnsupported = errors.New("resource does not support Kubernetes Table responses")
var ErrViewResponseTooLarge = errors.New("resource view response exceeds the size limit")

// ResourceViewClient reuses the authenticated, timeout-bound unary transport.
// Callers pass only validated resource identities, never remote URLs.
type ResourceViewClient struct{ client rest.Interface }

func (clients *Clients) ResourceViews() *ResourceViewClient {
	if clients == nil || clients.unary == nil || clients.unary.kubernetes == nil {
		return nil
	}
	return &ResourceViewClient{client: clients.unary.kubernetes.Discovery().RESTClient()}
}

func (client *ResourceViewClient) JSON(ctx context.Context, path string, query url.Values, accept string, into any) error {
	return client.JSONLimited(ctx, path, query, accept, into, maximumViewResponseBytes)
}

func (client *ResourceViewClient) JSONLimited(ctx context.Context, path string, query url.Values, accept string, into any, limit int64) error {
	if client == nil || client.client == nil {
		return errors.New("resource discovery client unavailable")
	}
	request := client.client.Get().AbsPath(path).SetHeader("Accept", accept)
	for key, values := range query {
		for _, value := range values {
			request.Param(key, value)
		}
	}
	stream, err := request.Stream(ctx)
	if err != nil {
		return fmt.Errorf("read resource view: %w", err)
	}
	defer stream.Close()
	data, err := io.ReadAll(io.LimitReader(stream, limit+1))
	if err != nil {
		return fmt.Errorf("read resource view response: %w", err)
	}
	if int64(len(data)) > limit {
		return ErrViewResponseTooLarge
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(into); err != nil {
		return errors.New("resource view returned an invalid response")
	}
	return nil
}

func (client *ResourceViewClient) Table(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string, options metav1.ListOptions) (*metav1.Table, error) {
	path := resourceViewPath(gvr, namespace, name)
	query := url.Values{"includeObject": {"Metadata"}}
	if name == "" {
		query.Set("limit", strconv.FormatInt(options.Limit, 10))
		if options.Continue != "" {
			query.Set("continue", options.Continue)
		}
		if options.LabelSelector != "" {
			query.Set("labelSelector", options.LabelSelector)
		}
		if options.FieldSelector != "" {
			query.Set("fieldSelector", options.FieldSelector)
		}
	}
	var table metav1.Table
	if err := client.JSON(ctx, path, query, "application/json;as=Table;g=meta.k8s.io;v=v1", &table); err != nil {
		return nil, err
	}
	if table.Kind != "Table" || table.APIVersion != "meta.k8s.io/v1" {
		return nil, ErrTableUnsupported
	}
	return &table, nil
}

func resourceViewPath(gvr schema.GroupVersionResource, namespace, name string) string {
	path := "/apis/" + gvr.Group + "/" + gvr.Version
	if gvr.Group == "" {
		path = "/api/" + gvr.Version
	}
	if namespace != "" {
		path += "/namespaces/" + namespace
	}
	path += "/" + gvr.Resource
	if name != "" {
		path += "/" + name
	}
	return path
}

func (client *ResourceViewClient) Document(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string, into any) error {
	return client.JSONLimited(ctx, resourceViewPath(gvr, namespace, name), url.Values{}, "application/json", into, 2<<20)
}
