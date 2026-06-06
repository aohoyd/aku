package k8s

import (
	"net/url"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

// execURL builds the /exec subresource request URL for a pod/container.
func execURL(typed kubernetes.Interface, podName, containerName, namespace string, command []string) *url.URL {
	return typed.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Stdin:     true,
			Stdout:    true,
			TTY:       true,
			Container: containerName,
			Command:   command,
		}, scheme.ParameterCodec).
		URL()
}

// NewExecExecutor builds a SPDY executor for exec-ing into a pod/container.
// The returned executor satisfies the minimal interface consumed by the
// session package and can be driven over in-memory pipes.
func NewExecExecutor(client *Client, podName, containerName, namespace string, command []string) (remotecommand.Executor, error) {
	reqURL := execURL(client.Typed, podName, containerName, namespace, command)
	return remotecommand.NewSPDYExecutor(client.Config, "POST", reqURL)
}

