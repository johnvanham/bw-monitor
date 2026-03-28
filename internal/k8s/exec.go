package k8s

import (
	"bytes"
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

// ExecInPod executes a command in a pod and returns stdout.
func (c *Client) ExecInPod(ctx context.Context, podName string, command []string) (string, error) {
	req := c.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(c.Namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Command: command,
			Stdout:  true,
			Stderr:  true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(c.Config, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("creating executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return "", fmt.Errorf("exec in pod %s: %w (stderr: %s)", podName, err, stderr.String())
	}

	return stdout.String(), nil
}

// ExecRedis runs a redis-cli command in the Redis pod.
func (c *Client) ExecRedis(ctx context.Context, podName string, args ...string) (string, error) {
	command := append([]string{"redis-cli", "--no-auth-warning"}, args...)
	return c.ExecInPod(ctx, podName, command)
}
