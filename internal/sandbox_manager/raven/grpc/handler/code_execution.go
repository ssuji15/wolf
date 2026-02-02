package handler

import (
	"fmt"
	"io"

	pb "github.com/ssuji15/wolf/cmd/wolf_worker/agent"
	"google.golang.org/grpc"
)

type CodeExecutionOutputStream struct {
	Stream grpc.ServerStreamingClient[pb.CodeExecutionOutput]
}

func (s *CodeExecutionOutputStream) Recv() ([]byte, error) {
	var data []byte
	for {
		msg, err := s.Stream.Recv()
		if err != nil {
			if err == io.EOF {
				return data, nil
			}
			return nil, fmt.Errorf("error receiving output: %v", err)
		}
		data = append(data, msg.Data...)
	}
}
