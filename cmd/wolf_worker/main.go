package main

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"time"

	pb "github.com/ssuji15/wolf/cmd/wolf_worker/agent"

	"google.golang.org/grpc"
)

const (
	jobDirectory = "/job"
	socketPath   = jobDirectory + "/socket/socket.sock"
	outputPath   = jobDirectory + "/output/output.log"
)

type WorkerAgent struct {
	pb.UnimplementedWorkerAgentServer
}

var grpcServer *grpc.Server

func main() {

	lis, err := net.Listen("unix", socketPath)
	if err != nil {
		panic(err)
	}

	grpcServer = grpc.NewServer()
	pb.RegisterWorkerAgentServer(grpcServer, &WorkerAgent{})

	fmt.Println("worker listening on", socketPath)
	grpcServer.Serve(lis)
}

func (w *WorkerAgent) StartJob(ctx context.Context, req *pb.JobRequest) (*pb.Ack, error) {
	go func() {
		err := runJob(req.Engine, req.Code)
		if err != nil {
			f, ferr := os.OpenFile(outputPath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
			if ferr != nil {
				os.Exit(1)
			}
			defer f.Close()
			if _, werr := f.Write([]byte("\n=== ERR ===\n")); werr != nil {
				os.Exit(1)
			}
			if _, werr := f.Write([]byte(err.Error())); werr != nil {
				os.Exit(1)
			}
			os.Exit(1)
		}
		os.Exit(0)
	}()
	return &pb.Ack{Message: "ok"}, nil
}

func runJob(engine, code string) error {
	src := getFileName(engine)
	f, _ := os.Create(outputPath)
	f.Close()
	switch engine {
	case "c++":
		re := regexp.MustCompile(`(?m)^#include\s+<.*>$`)
		cleanCode := re.ReplaceAllString(code, "")
		finalCode := "#include <bits/stdc++.h>\n" + cleanCode
		err := os.WriteFile(src, []byte(finalCode), 0644)
		if err != nil {
			return err
		}

		if err := run("clang++", "-O1", "-pipe", "-include-pch", "/usr/include/c++/12/bits/stdc++.h.pch", src, "-o", jobDirectory+"/prog"); err != nil {
			return err
		}
		if err := run(jobDirectory + "/prog"); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown engine")
	}
	return nil
}

func getFileName(engine string) string {
	switch engine {
	case "c++":
		return jobDirectory + "/p.cpp"
	case "java":
		return jobDirectory + "/p.java"
	default:
		return jobDirectory + "/p.txt"
	}
}

func run(cmd string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	c := exec.CommandContext(ctx, cmd, args...)
	var stdout bytes.Buffer
	c.Stdout = &stdout
	err := c.Run()
	if err != nil {
		return fmt.Errorf("command failed: %w", err)
	}

	// Truncate output to 1MB
	outBytes := stdout.Bytes()
	const max = 1024 * 1024
	if len(outBytes) > max {
		outBytes = outBytes[:max]
	}

	if len(outBytes) > 0 {
		f, ferr := os.OpenFile(outputPath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
		if ferr != nil {
			return ferr
		}
		defer f.Close()
		if _, werr := f.Write([]byte("=== STDOUT ===\n")); werr != nil {
			return werr
		}
		if _, werr := f.Write(outBytes); werr != nil {
			return werr
		}
	}
	return nil
}
