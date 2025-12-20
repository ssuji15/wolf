package web

import (
	pb "github.com/ssuji15/wolf-worker/agent"
)

type BackendReceiver struct {
	pb.UnimplementedWorkerAgentServer
}

// func (b *BackendReceiver) UploadResult(ctx context.Context, response *pb.JobResponse) (*pb.Ack, error) {
// 	span := trace.SpanFromContext(ctx)

// 	js := jobservice.GetJobService()
// 	comp := component.GetComponent()
// 	job, err := js.GetJob(ctx, response.JobId)
// 	if err != nil {
// 		if span.IsRecording() {
// 			span.RecordError(err)
// 			span.SetStatus(codes.Error, err.Error())
// 		}
// 		return nil, fmt.Errorf("failed to retrieve job: %v", err)
// 	}

// 	o := []byte(response.Output)
// 	hashBytes := sha256.Sum256(o)
// 	oHash := fmt.Sprintf("%x", hashBytes[:])
// 	objectPath := util.GetOutputPath(oHash)
// 	if err := comp.StorageClient.Upload(ctx, objectPath, o); err != nil {
// 		return nil, fmt.Errorf("failed to upload code: %v", err)
// 	}
// 	now := time.Now().UTC()
// 	job.OutputHash = oHash
// 	job.Status = string(jobservice.JOB_COMPLETED)
// 	job.EndTime = &now

// 	err = js.UpdateJob(ctx, job)
// 	if err != nil {
// 		return nil, fmt.Errorf("unable to update job in db: %v", err)
// 	}

// 	return &pb.Ack{Message: "received"}, nil
// }
