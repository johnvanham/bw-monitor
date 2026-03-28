package k8s

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client wraps a Kubernetes clientset and REST config.
type Client struct {
	Clientset *kubernetes.Clientset
	Config    *rest.Config
	Namespace string
}

// NewClient creates a Kubernetes client from the current kubeconfig context.
func NewClient(namespace string) (*Client, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	config := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, nil)

	restConfig, err := config.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("loading kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes client: %w", err)
	}

	return &Client{
		Clientset: clientset,
		Config:    restConfig,
		Namespace: namespace,
	}, nil
}

// FindRedisPod discovers the BunkerWeb Redis pod by label selector.
func (c *Client) FindRedisPod(ctx context.Context) (string, error) {
	pods, err := c.Clientset.CoreV1().Pods(c.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "bunkerweb.io/component=redis",
	})
	if err != nil {
		return "", fmt.Errorf("listing redis pods: %w", err)
	}
	if len(pods.Items) == 0 {
		return "", fmt.Errorf("no redis-bunkerweb pods found in namespace %s", c.Namespace)
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase == "Running" {
			return pod.Name, nil
		}
	}
	return "", fmt.Errorf("no running redis-bunkerweb pods found")
}
