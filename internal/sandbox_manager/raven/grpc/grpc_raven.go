package grpc

import (
	"context"
	"fmt"

	pb "github.com/ssuji15/wolf/cmd/wolf_worker/agent"
	"github.com/ssuji15/wolf/internal/sandbox_manager/raven/grpc/handler"
	"github.com/ssuji15/wolf/internal/sandbox_manager/raven/grpc/transport"
	"github.com/ssuji15/wolf/model"
	"google.golang.org/grpc"
)

type OutputStream interface {
	Recv() ([]byte, error)
}

type GRPCRavenJobType string

const (
	SendCode GRPCRavenJobType = "codeExecution"
)

type GRPCRaven struct {
	Transport transport.Transport
	Type      GRPCRavenJobType
	OStream   OutputStream
}

type Option struct {
	TransportOption transport.Option
	Type            GRPCRavenJobType
}

func NewRaven(opt Option) (*GRPCRaven, error) {
	t, err := transport.NewTransport(opt.TransportOption)
	if err != nil {
		return nil, err
	}
	return &GRPCRaven{
		Transport: t,
		Type:      opt.Type,
	}, nil
}

func (r *GRPCRaven) Close() error {
	return nil
}

func (r *GRPCRaven) Send(ctx context.Context, job *model.Job, payload []byte) error {

	conn, err := r.Transport.Dial(ctx)
	if err != nil {
		return err
	}

	switch r.Type {
	case SendCode:
		return r.sendCode(ctx, job.ExecutionEngine, payload, conn)
	default:
		return fmt.Errorf("invalid UDSRaven Type")
	}
}

func (r *GRPCRaven) Receive(ctx context.Context) ([]byte, error) {
	return r.OStream.Recv()
}

func (r *GRPCRaven) sendCode(ctx context.Context, engine string, payload []byte, conn *grpc.ClientConn) error {
	client := pb.NewWorkerAgentClient(conn)
	stream, err := client.StartJob(ctx, &pb.JobRequest{
		Engine: engine,
		Code:   string(payload),
	})
	r.OStream = &handler.CodeExecutionOutputStream{
		Stream: stream,
	}
	return err
}
