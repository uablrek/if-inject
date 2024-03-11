/*
SPDX-License-Identifier: CC0-1.0
https://creativecommons.org/publicdomain/zero/1.0/
*/
package util

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"time"

	"github.com/go-logr/logr"
	"google.golang.org/grpc"
	cri "k8s.io/cri-api/pkg/apis/runtime/v1"
)

type RuntimeConnection struct {
	io.Closer
	conn   *grpc.ClientConn
	client cri.RuntimeServiceClient
}

// NewRuntimeConnection Create a connection to the runtime
// The socket is searched in order from;
//   - The passed uri
//   - $CONTAINER_RUNTIME_ENDPOINT
//   - unix:///run/crio/crio.sock
//   - unix:///run/containerd/containerd.sock
//   - unix:///var/run/crio/crio.sock
func NewRuntimeConnection(
	ctx context.Context, uri string) (*RuntimeConnection, error) {
	searchURLs := []string{
		uri,
		os.Getenv("CONTAINER_RUNTIME_ENDPOINT"),
		"unix:///run/crio/crio.sock",
		"unix:///run/containerd/containerd.sock",
		"unix:///var/run/crio/crio.sock",
	}
	logger := logr.FromContextOrDiscard(ctx).V(2)
	for _, u := range searchURLs {
		if u == "" {
			continue
		}
		conn, err := doConnect(ctx, u)
		if err == nil {
			return conn, nil
		}
		if logger.Enabled() {
			logger.Error(err, "runtime", "path", u)
		}
	}
	return nil, fmt.Errorf("Runtime not found")
}
func (conn *RuntimeConnection) Close() error {
	return conn.conn.Close()
}

func doConnect(ctx context.Context, URL string) (*RuntimeConnection, error) {
	logger := logr.FromContextOrDiscard(ctx)
	u, err := url.Parse(URL)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "unix" {
		return nil, fmt.Errorf("Invalid Scheme %s", u.Scheme)
	}
	stat, err := os.Stat(u.Path)
	if err != nil {
		return nil, err
	}
	if (stat.Mode() & os.ModeSocket) == 0 {
		logger.Info("filemode", "path", u.Path, "mode", stat.Mode())
		return nil, fmt.Errorf("Not a unix socket %s", u.Path)
	}
	logger.V(1).Info("RuntimeConnection", "endpoint", URL)
	conn := RuntimeConnection{}
	conn.conn, err = grpc.DialContext(
		ctx, URL, grpc.WithInsecure(), grpc.WithBlock(),
		grpc.WithTimeout(10*time.Second))
	if err != nil {
		return nil, err
	}
	conn.client = cri.NewRuntimeServiceClient(conn.conn)
	return &conn, nil
}

// GetPodSandbox Returns the runtime PodSandbox for a K8s namespace/pod
func GetPodSandbox(
	ctx context.Context, conn *RuntimeConnection, pod, ns string) (
	*cri.PodSandbox, error) {
	request := &cri.ListPodSandboxRequest{
		Filter: &cri.PodSandboxFilter{
			LabelSelector: map[string]string{
				"io.kubernetes.pod.name":      pod,
				"io.kubernetes.pod.namespace": ns,
			},
		},
	}
	resp, err := conn.client.ListPodSandbox(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("ListPodSandbox %w", err)
	}
	if len(resp.Items) == 0 {
		return nil, fmt.Errorf("PodSandbox not found")
	}
	if len(resp.Items) != 1 {
		return nil, fmt.Errorf("Unexpected matches %d", len(resp.Items))
	}
	return resp.Items[0], nil
}

// GetNetns Returns the Linux netns path for a K8s namespace/pod
func GetNetns(ctx context.Context, pod, ns string) (string, error) {
	/*
	   The best way seem to be to read the PodSandboxStatus from the
	   runtime. It can be done manually with

	     crictl pods
	     crictl inspectp -o json NON-HOST-NETWORK-PODID \
	       | jq ".info.runtimeSpec.linux.namespaces" \
	       | jq -r '.[] | select(.type == "network") | .path'

	   Please see https://github.com/containerd/containerd/issues/9838.

	   A problem is that the runtime POD-ID must be used, which is not
	   known by K8s. However the PodSandbox has labels
	   "io.kubernetes.pod.name" and "io.kubernetes.pod.namespace" that
	   can be used as a filter. The K8s API server is *not* involved.
	*/
	logger := logr.FromContextOrDiscard(ctx)
	conn, err := NewRuntimeConnection(ctx, "")
	if err != nil {
		return "", fmt.Errorf("NewRuntimeConnection: %w", err)
	}
	podSandbox, err := GetPodSandbox(ctx, conn, pod, ns)
	if err != nil {
		return "", fmt.Errorf("GetPodSandbox: %w", err)
	}
	defer conn.Close()
	logger.V(1).Info("PodSandbox", "id", podSandbox.Id)

	request := &cri.PodSandboxStatusRequest{
		PodSandboxId: podSandbox.Id,
		Verbose:      true, // request the "Info" map
	}
	resp, err := conn.client.PodSandboxStatus(ctx, request)
	if err != nil {
		return "", fmt.Errorf("PodSandboxStatus: %w", err)
	}

	info := resp.GetInfo()
	var infop any
	err = json.Unmarshal([]byte(info["info"]), &infop)
	if err != nil {
		return "", fmt.Errorf("Unmarshal info %w", err)
	}
	infomap := infop.(map[string]any)
	// To see what we get from the runtime:
	// if-inject -loglevel 4 netns -ns if-inject -pod $pod 2> log
	// cat log | jq
	logger.V(4).Info("Pod info", "infomap", infomap)
	// TODO: errorhandling on type-casts
	namespaces := infomap["runtimeSpec"].(map[string]any)["linux"].(map[string]any)["namespaces"].([]any)
	logger.V(2).Info("namespaces reported by cri", "namespaces", namespaces)
	for _, ns := range namespaces {
		nsobj := ns.(map[string]any)
		if nsobj["type"] == "network" {
			// TODO: errorhandling on lookup and cast
			return nsobj["path"].(string), nil
		}
	}

	return "", fmt.Errorf("Namespace path not found")
}
