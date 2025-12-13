package containerdservice

import (
	"github.com/containerd/containerd"
)

func NewContainerdClient() (*containerd.Client, error) {
	return containerd.New(
		"/run/containerd/containerd.sock",
		containerd.WithDefaultNamespace("default"),
	)
}
