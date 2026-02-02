package transport

import (
	"context"

	"google.golang.org/grpc"
)

type TCPTransport struct {
	Addr string
}

func NewTCPTransport(a string) *TCPTransport {
	return &TCPTransport{
		Addr: a,
	}
}

func (t *TCPTransport) Dial(ctx context.Context) (*grpc.ClientConn, error) {
	return grpc.DialContext(
		ctx,
		t.Addr,
		grpc.WithInsecure(),
	)
}
