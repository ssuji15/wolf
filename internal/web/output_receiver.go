package web

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	pb "github.com/ssuji15/wolf-worker/agent"
	"github.com/ssuji15/wolf/internal/component"
	jobservice "github.com/ssuji15/wolf/internal/service/job_service"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type BackendReceiver struct {
	pb.UnimplementedWorkerAgentServer
}

func (b *BackendReceiver) UploadResult(ctx context.Context, response *pb.JobResponse) (*pb.Ack, error) {
	span := trace.SpanFromContext(ctx)

	js := jobservice.GetJobService()
	comp := component.GetComponent()
	job, err := js.GetJob(ctx, response.JobId)
	if err != nil {
		if span.IsRecording() {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		return nil, fmt.Errorf("failed to retrieve job: %v", err)
	}

	o := []byte(response.Output)
	objectPath := fmt.Sprintf("jobs/%s/output.log", response.JobId)
	if err := comp.StorageClient.Upload(ctx, objectPath, o); err != nil {
		return nil, fmt.Errorf("failed to upload code: %v", err)
	}

	hashBytes := sha256.Sum256(o)
	oHash := fmt.Sprintf("%x", hashBytes[:])
	now := time.Now()

	job.OutputPath = objectPath
	job.OutputHash = oHash
	job.Status = string(jobservice.JobCompleted)
	job.EndTime = &now

	err = js.UpdateJob(ctx, job)
	if err != nil {
		return nil, fmt.Errorf("unable to update job in db: %v", err)
	}

	return &pb.Ack{Message: "received"}, nil
}
