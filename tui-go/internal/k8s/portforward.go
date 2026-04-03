package k8s

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// PortForward holds a running port-forward connection to a pod.
type PortForward struct {
	LocalPort uint16
	stopChan  chan struct{}
	fw        *portforward.PortForwarder
}

// StartPortForward opens a port-forward to a pod and returns the local port.
// The caller must call Close() when done.
func (c *Client) StartPortForward(podName string, remotePort int) (*PortForward, error) {
	// Find a free local port
	localPort, err := freePort()
	if err != nil {
		return nil, fmt.Errorf("finding free port: %w", err)
	}

	// Build the portforward URL
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", c.Namespace, podName)
	hostURL, err := url.Parse(c.Config.Host)
	if err != nil {
		return nil, fmt.Errorf("parsing host URL: %w", err)
	}
	hostURL.Path = path

	transport, upgrader, err := spdy.RoundTripperFor(c.Config)
	if err != nil {
		return nil, fmt.Errorf("creating round tripper: %w", err)
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", hostURL)

	stopChan := make(chan struct{}, 1)
	readyChan := make(chan struct{}, 1)

	ports := []string{fmt.Sprintf("%d:%d", localPort, remotePort)}
	fw, err := portforward.New(dialer, ports, stopChan, readyChan, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("creating port-forwarder: %w", err)
	}

	// Start in background
	errChan := make(chan error, 1)
	go func() {
		errChan <- fw.ForwardPorts()
	}()

	// Wait for ready or error
	select {
	case <-readyChan:
		// Get the actual local port (in case it changed)
		forwardedPorts, err := fw.GetPorts()
		if err != nil {
			close(stopChan)
			return nil, fmt.Errorf("getting forwarded ports: %w", err)
		}
		if len(forwardedPorts) == 0 {
			close(stopChan)
			return nil, fmt.Errorf("no ports forwarded")
		}
		return &PortForward{
			LocalPort: forwardedPorts[0].Local,
			stopChan:  stopChan,
			fw:        fw,
		}, nil
	case err := <-errChan:
		if err != nil {
			// Check if it's a "use of closed network connection" from stopChan
			if strings.Contains(err.Error(), "use of closed network connection") {
				return nil, fmt.Errorf("port-forward failed to start")
			}
			return nil, fmt.Errorf("port-forward: %w", err)
		}
		return nil, fmt.Errorf("port-forward exited unexpectedly")
	}
}

// Close stops the port-forward.
func (pf *PortForward) Close() {
	close(pf.stopChan)
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
