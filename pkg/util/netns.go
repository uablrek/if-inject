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
	k8s "k8s.io/api/core/v1"
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

// GetNetns Returns the netns and containerID for a POD
func GetNetns(ctx context.Context, pod *k8s.Pod) (string, string, error) {
	/*
	   There doesn't seem to be a defined way, or best practice for
	   this. The netns is not known in K8s, but in the CRI-plugin
	   (e.g. cri-o or containerd). The safest way seem to be to get
	   the pid of a running container and return "/proc/pid/ns/net".
	*/
	logger := logr.FromContextOrDiscard(ctx)
	logger.V(4).Info("GetNetns", "pod", *pod)

	// Find a running container, and get the ContainerID
	var u *url.URL
	var err error
	for _, c := range pod.Status.ContainerStatuses {
		if c.State.Running == nil {
			continue
		}
		u, err = url.Parse(c.ContainerID)
		if err != nil {
			return "", "", err
		}
		break
	}
	if u == nil {
		return "", "", fmt.Errorf("No running container found")
	}
	logger.V(2).Info("ContainerID", "url", u)

	// Get the ContainerStatus from the runtime
	conn, err := NewRuntimeConnection(ctx, "")
	if err != nil {
		return "", "", err
	}
	defer conn.Close()
	request := &cri.ContainerStatusRequest{
		ContainerId: u.Host,
		Verbose:     true,		// request the "Info" map
	}
	r, err := conn.client.ContainerStatus(ctx, request)
	if err != nil {
		return "", "", err
	}

	// Read the "Info" map
	info := r.GetInfo()
	var infop any
	err = json.Unmarshal([]byte(info["info"]), &infop)
	if err != nil {
		return "", "", err
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
			return nsobj["path"].(string), u.Host, nil
		}
	}
	// Fallback to pid if no network namespace is found
	pid, ok := infomap["pid"].(float64)
	if !ok {
		return "", "", fmt.Errorf("cannot get pid from containerStatus info")
	}
	return fmt.Sprintf("/proc/%d/ns/net", int(pid)), u.Host, nil
}
