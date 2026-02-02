package raven

import (
	"context"
	"fmt"

	"github.com/ssuji15/wolf/internal/sandbox_manager/raven/grpc"
	"github.com/ssuji15/wolf/internal/sandbox_manager/raven/grpc/transport"
	"github.com/ssuji15/wolf/model"
)

type Raven interface {
	Send(ctx context.Context, job *model.Job, payload []byte) error
	Receive(ctx context.Context) ([]byte, error)
	Close() error
}

type Type string

const (
	RavenGRPC Type = "grpc"
	RavenFile Type = "file"
)

type Option struct {
	GrpcJobType         grpc.GRPCRavenJobType
	GrpcTransportOption transport.Option
}

func NewRaven(r Type, opt Option) (Raven, error) {
	switch r {
	case RavenGRPC:
		return grpc.NewRaven(grpc.Option{
			Type:            opt.GrpcJobType,
			TransportOption: opt.GrpcTransportOption,
		})
	default:
		return nil, fmt.Errorf("unsupported raven typeeee: %s", string(r))
	}
}
