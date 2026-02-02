package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	pb "github.com/ssuji15/wolf/cmd/wolf_worker/agent"
	"github.com/ssuji15/wolf/internal/sandbox_manager/worker"

	"google.golang.org/grpc"
)

const (
	tmpfsDirectory = "/tmp"
	jobDirectory   = "/job"
	socketPath     = jobDirectory + "/socket.sock"
	maxOutputBytes = 1 * 1024 * 1024 // 1 MB
)

type WorkerAgent struct {
	pb.UnimplementedWorkerAgentServer
}

var grpcServer *grpc.Server

func main() {

	transport := os.Getenv(worker.WORKER_TRANSPORT_ENV)
	if transport == "" {
		transport = "uds"
	}

	var (
		lis net.Listener
		err error
	)

	switch transport {
	case "uds":
		lis, err = net.Listen("unix", socketPath)
	case "tcp":
		addr := "0.0.0.0:" + worker.WORKER_LISTEN_PORT
		lis, err = net.Listen("tcp", addr)
	default:
		panic("unknown WORKER_TRANSPORT")
	}

	if err != nil {
		panic(err)
	}

	grpcServer = grpc.NewServer(grpc.MaxRecvMsgSize(2*1024*1024), grpc.MaxSendMsgSize(2*1024*1024))
	pb.RegisterWorkerAgentServer(grpcServer, &WorkerAgent{})

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-stopCh
		fmt.Println("received signal, stopping gRPC server:", sig)
		grpcServer.GracefulStop()
	}()

	grpcServer.Serve(lis)
}

func (w *WorkerAgent) StartJob(req *pb.JobRequest, stream pb.WorkerAgent_StartJobServer) error {

	ctx, cancel := context.WithTimeout(stream.Context(), 2*time.Second)
	defer cancel()
	err := runJobStreaming(ctx, req.Engine, req.Code, stream)
	go func() {
		fmt.Println("job complete, stopping gRPC server")
		grpcServer.Stop()
	}()
	return err
}

func runJobStreaming(ctx context.Context, engine, code string, stream pb.WorkerAgent_StartJobServer) error {

	src := getFileName(engine)

	if err := os.WriteFile(src, []byte(code), 0644); err != nil {
		return err
	}

	//try "-fuse-ld=lld"

	var cmd *exec.Cmd
	switch engine {
	case "c++":
		cmd = exec.CommandContext(
			ctx,
			"clang++",
			"-O1",
			"-fno-exceptions",
			"-fno-rtti",
			"-pipe",
			"-include-pch",
			"/usr/include/c++/12/bits/stdc++.h.pch",
			src,
			"-o",
			tmpfsDirectory+"/prog",
		)
	default:
		return fmt.Errorf("unknown engine")
	}

	var sent int64

	// Compile code
	if err := streamCmdOutput(cmd, stream, &sent); err != nil {
		return nil
	}

	// Run compiled program
	cmd = exec.CommandContext(ctx, tmpfsDirectory+"/prog")
	streamCmdOutput(cmd, stream, &sent)
	return nil
}

func getFileName(engine string) string {
	switch engine {
	case "c++":
		return tmpfsDirectory + "/p.cpp"
	case "java":
		return tmpfsDirectory + "/p.java"
	default:
		return tmpfsDirectory + "/p.txt"
	}
}

func streamCmdOutput(cmd *exec.Cmd, stream pb.WorkerAgent_StartJobServer, sent *int64) error {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	send := func(kind pb.CodeExecutionOutput_Stream, data []byte) error {
		if err := stream.Context().Err(); err != nil {
			return err
		}

		if *sent+int64(len(data)) > maxOutputBytes {
			return fmt.Errorf("output limit exceeded")
		}
		*sent += int64(len(data))

		return stream.Send(&pb.CodeExecutionOutput{
			Stream: kind,
			Data:   data,
		})
	}

	errCh := make(chan error, 2)

	go readAndStream(stdout, pb.CodeExecutionOutput_STDOUT, send, errCh)
	go readAndStream(stderr, pb.CodeExecutionOutput_STDERR, send, errCh)

	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			_ = cmd.Process.Kill()
			return err
		}
	}

	return cmd.Wait()
}

func readAndStream(r io.Reader, kind pb.CodeExecutionOutput_Stream, send func(pb.CodeExecutionOutput_Stream, []byte) error, errCh chan<- error) {

	buf := make([]byte, 32*1024) // 32 KB chunks

	for {
		n, err := r.Read(buf)
		if n > 0 {
			if err := send(kind, buf[:n]); err != nil {
				errCh <- err
				return
			}
		}
		if err != nil {
			if err != io.EOF {
				errCh <- err
			} else {
				errCh <- nil
			}
			return
		}
	}
}
