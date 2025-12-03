package web

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	pb "github.com/ssuji15/wolf-worker/agent"
	"github.com/ssuji15/wolf/internal/component"
	"github.com/ssuji15/wolf/internal/service"
)

type BackendReceiver struct {
	pb.UnimplementedWorkerAgentServer
}

func (b *BackendReceiver) UploadResult(ctx context.Context, response *pb.JobResponse) (*pb.Ack, error) {

	jobservice := service.GetJobService()
	comp := component.GetComponent()
	job, err := jobservice.GetJob(ctx, response.JobId)
	if err != nil {
		fmt.Printf("failed to retrieve job: %v", err)
		return nil, fmt.Errorf("failed to retrieve job: %v", err)
	}

	o := []byte(response.Output)
	objectPath := fmt.Sprintf("jobs/%s/output.log", response.JobId)
	if err := comp.StorageClient.Upload(ctx, objectPath, o); err != nil {
		fmt.Printf("failed to upload code to minio: %v", err)
		return nil, fmt.Errorf("failed to upload code to minio: %v", err)
	}

	hashBytes := sha256.Sum256(o)
	oHash := fmt.Sprintf("%x", hashBytes[:])
	now := time.Now()

	job.OutputPath = objectPath
	job.OutputHash = oHash
	job.Output = response.Output
	job.Status = "Completed"
	job.EndTime = &now

	err = jobservice.UpdateJob(ctx, job)
	if err != nil {
		fmt.Printf("unable to update job: %v", err)
		return nil, fmt.Errorf("unable to update job: %v", err)
	}

	return &pb.Ack{Message: "received"}, nil
}
