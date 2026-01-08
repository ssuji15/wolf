package main

import (
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
			os.WriteFile(outputPath, []byte(fmt.Sprintf("%v", err)), 0644)
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
		os.WriteFile(src, []byte(finalCode), 0644)

		if err := run("clang++", "-O1", "-pipe", "-include-pch", "/usr/include/c++/12/bits/stdc++.h.pch", src, "-o", jobDirectory+"/prog"); err != nil {
			return err
		}
		if err := run(jobDirectory + "/prog"); err != nil {
			return err
		}
	default:
		os.WriteFile(outputPath, []byte("unknown engine"), 0644)
		return nil
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c := exec.CommandContext(ctx, cmd, args...)
	out, err := c.CombinedOutput()

	if err != nil {
		fmt.Println("failed to execute command:", err)
		return err
	}

	if len(out) > 1024*1024 {
		out = out[:1024*1024]
	}

	f, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(out); err != nil {
		return err
	}
	return nil
}
