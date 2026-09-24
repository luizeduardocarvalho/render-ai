package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	taskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	"google.golang.org/protobuf/types/known/durationpb"
)

// maxDispatchDeadline is Cloud Tasks' own ceiling for an HTTP task's
// dispatch deadline.
const maxDispatchDeadline = 30 * time.Minute

// CloudTasks is the Queue used with the "cloudtasks" jobs.queue config: each
// Enqueue creates one Cloud Tasks HTTP task that calls the worker route
// directly on this same Cloud Run service (bypassing Firebase Hosting's 60s
// cutoff), carrying a Google-minted OIDC
// token the worker route verifies (see internal/api.Server.verifyWorkerRequest).
type CloudTasks struct {
	client           *cloudtasks.Client
	queuePath        string // projects/P/locations/L/queues/Q
	workerURL        string // e.g. https://render-ai-api-xxx.a.run.app (no path)
	invokerSA        string
	audience         string
	dispatchDeadline time.Duration
}

// NewCloudTasks builds a CloudTasks queue. renderTimeoutSec is the same
// per-render timeout the worker itself applies to the Vertex AI call; the
// dispatch deadline is set to renderTimeoutSec+120s (capped at Cloud Tasks'
// own 30-minute ceiling) so Cloud Tasks doesn't give up on - and the worker
// process doesn't get killed mid - a slow render.
func NewCloudTasks(ctx context.Context, queuePath, workerURL, invokerSA, audience string, renderTimeoutSec int) (*CloudTasks, error) {
	client, err := cloudtasks.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating cloud tasks client: %w", err)
	}
	deadline := time.Duration(renderTimeoutSec+120) * time.Second
	if deadline > maxDispatchDeadline {
		deadline = maxDispatchDeadline
	}
	return &CloudTasks{
		client:           client,
		queuePath:        queuePath,
		workerURL:        workerURL,
		invokerSA:        invokerSA,
		audience:         audience,
		dispatchDeadline: deadline,
	}, nil
}

// Enqueue creates one Cloud Tasks HTTP task for task, targeting
// {workerURL}/internal/render-tasks with an OIDC-authenticated POST.
func (q *CloudTasks) Enqueue(ctx context.Context, task Task) error {
	body, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("encoding render task: %w", err)
	}
	req := &taskspb.CreateTaskRequest{
		Parent: q.queuePath,
		Task: &taskspb.Task{
			DispatchDeadline: durationpb.New(q.dispatchDeadline),
			MessageType: &taskspb.Task_HttpRequest{
				HttpRequest: &taskspb.HttpRequest{
					Url:        q.workerURL + "/internal/render-tasks",
					HttpMethod: taskspb.HttpMethod_POST,
					Headers:    map[string]string{"Content-Type": "application/json"},
					Body:       body,
					AuthorizationHeader: &taskspb.HttpRequest_OidcToken{
						OidcToken: &taskspb.OidcToken{
							ServiceAccountEmail: q.invokerSA,
							Audience:            q.audience,
						},
					},
				},
			},
		},
	}
	if _, err := q.client.CreateTask(ctx, req); err != nil {
		return fmt.Errorf("creating cloud task: %w", err)
	}
	return nil
}
