package k8s

import (
	"context"
	"io"
	"net/url"
	"testing"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func TestContextPropagationSignatures(t *testing.T) {
	// Compile-time verification that execContainer, attachContainer, and spdyStream
	// all accept context.Context as their first parameter. If someone removes the
	// context parameter, this test will fail to compile.
	var (
		_ func(context.Context, io.Reader, io.Writer, io.Writer, *rest.Config, kubernetes.Interface, string, string, string, []string) error = execContainer
		_ func(context.Context, io.Reader, io.Writer, io.Writer, *rest.Config, kubernetes.Interface, string, string, string) error            = attachContainer
		_ func(context.Context, io.Reader, io.Writer, *rest.Config, *url.URL) error                                                          = spdyStream
	)
}
