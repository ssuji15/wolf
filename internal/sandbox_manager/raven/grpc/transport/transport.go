package transport

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
)

type Transport interface {
	Dial(ctx context.Context) (*grpc.ClientConn, error)
}

type TransportType string

const (
	UDS TransportType = "uds"
	TCP TransportType = "tcp"
)

type Option struct {
	Path string
	Type TransportType
}

func NewTransport(opt Option) (Transport, error) {
	switch opt.Type {
	case UDS:
		return NewUDSTransport(opt.Path)
	case TCP:
		return NewTCPTransport(opt.Path), nil
	default:
		return nil, fmt.Errorf("unsupported transport type")
	}
}
