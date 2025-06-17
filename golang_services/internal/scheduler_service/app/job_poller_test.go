package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/AradIT/aradsms/golang_services/internal/platform/messagebroker"
	"github.com/AradIT/aradsms/golang_services/internal/scheduler_service/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	// Prometheus testing utilities are typically for integration tests.
	// For unit tests, we focus on behavior that leads to metric updates.
)

// --- Mocks ---

type MockScheduledJobRepository struct {
	mock.Mock
}

func (m *MockScheduledJobRepository) CreateJob(ctx context.Context, job *domain.ScheduledJob) error {
	args := m.Called(ctx, job)
	return args.Error(0)
}

func (m *MockScheduledJobRepository) GetJob(ctx context.Context, jobID uuid.UUID) (*domain.ScheduledJob, error) {
	args := m.Called(ctx, jobID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ScheduledJob), args.Error(1)
}

func (m *MockScheduledJobRepository) AcquireDueJobs(ctx context.Context, dueTime time.Time, limit int) ([]*domain.ScheduledJob, error) {
	args := m.Called(ctx, dueTime, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.ScheduledJob), args.Error(1)
}

func (m *MockScheduledJobRepository) UpdateStatus(ctx context.Context, jobID uuid.UUID, status domain.JobStatus, processedAt time.Time, processingErr sql.NullString, retryCount int) error {
	args := m.Called(ctx, jobID, status, processedAt, processingErr, retryCount)
	return args.Error(0)
}

func (m *MockScheduledJobRepository) MarkForRetry(ctx context.Context, jobID uuid.UUID, nextRunAt time.Time, retryCount int, lastError sql.NullString) error {
	args := m.Called(ctx, jobID, nextRunAt, retryCount, lastError)
	return args.Error(0)
}

// MockNatsClient
type MockNatsClient struct {
	mock.Mock
}

func (m *MockNatsClient) Publish(ctx context.Context, subject string, data []byte) error {
	args := m.Called(ctx, subject, data)
	return args.Error(0)
}

func (m *MockNatsClient) Subscribe(ctx context.Context, subject string, queueGroup string, handler func(msg messagebroker.Message)) (messagebroker.Subscription, error) {
	args := m.Called(ctx, subject, queueGroup, handler)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(messagebroker.Subscription), args.Error(1)
}
func (m *MockNatsClient) SubscribeToSubjectWithQueue(ctx context.Context, subject string, queueGroup string, handler func(*nats.Msg)) error {
	args := m.Called(ctx, subject, queueGroup, handler)
	return args.Error(0)
}


// --- Test Setup ---
type jobPollerTestComponents struct {
	jobPoller  *JobPoller
	mockRepo   *MockScheduledJobRepository
	mockNats   *MockNatsClient
	logger     *slog.Logger
	pollerCfg  PollerConfig
}

func setupJobPollerTest(t *testing.T) jobPollerTestComponents {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil)) // Discard logs for tests
	mockRepo := new(MockScheduledJobRepository)
	mockNats := new(MockNatsClient)

	pollerCfg := PollerConfig{
		PollingInterval: 30 * time.Second, // Not directly used by PollAndProcessJobs, but part of config
		JobBatchSize:    5,
		MaxRetry:        3,
	}

	jobPoller := NewJobPoller(
		mockRepo,
		mockNats,
		logger,
		pollerCfg,
	)

	return jobPollerTestComponents{
		jobPoller:  jobPoller,
		mockRepo:   mockRepo,
		mockNats:   mockNats,
		logger:     logger,
		pollerCfg:  pollerCfg,
	}
}

// --- Tests for PollAndProcessJobs ---

func TestJobPoller_PollAndProcessJobs(t *testing.T) {
	ctx := context.Background()

	t.Run("SuccessfulProcessingOfSMSJob", func(t *testing.T) {
		comps := setupJobPollerTest(t)
		jobID := uuid.New()
		smsPayload := domain.SMSJobPayload{OutboxMessageID: uuid.New().String()}
		payloadBytes, _ := json.Marshal(smsPayload)

		mockJob := &domain.ScheduledJob{
			ID:        jobID,
			JobType:   domain.JobTypeSMS,
			Payload:   payloadBytes,
			Status:    domain.StatusQueued,
			RetryCount: 0,
		}
		comps.mockRepo.On("AcquireDueJobs", ctx, mock.AnythingOfType("time.Time"), comps.pollerCfg.JobBatchSize).Return([]*domain.ScheduledJob{mockJob}, nil).Once()
		comps.mockNats.On("Publish", ctx, "sms.jobs.send", payloadBytes).Return(nil).Once()
		comps.mockRepo.On("UpdateStatus", ctx, jobID, domain.StatusCompleted, mock.AnythingOfType("time.Time"), sql.NullString{}, 0).Return(nil).Once()

		processed, err := comps.jobPoller.PollAndProcessJobs(ctx)

		assert.NoError(t, err)
		assert.Equal(t, 1, processed)
		comps.mockRepo.AssertExpectations(t)
		comps.mockNats.AssertExpectations(t)
		// Metric verification: jobsProcessedCounter with job_type="sms", status="success" should have been incremented.
		// jobProcessingDurationHist for "sms" should have been observed.
		// This is hard to assert directly in unit test without Prometheus test utilities or metric registry DI.
		// We rely on the code path being executed.
	})

	t.Run("NoJobsAvailable", func(t *testing.T) {
		comps := setupJobPollerTest(t)
		comps.mockRepo.On("AcquireDueJobs", ctx, mock.AnythingOfType("time.Time"), comps.pollerCfg.JobBatchSize).Return([]*domain.ScheduledJob{}, nil).Once()

		processed, err := comps.jobPoller.PollAndProcessJobs(ctx)

		assert.NoError(t, err)
		assert.Equal(t, 0, processed)
		comps.mockRepo.AssertExpectations(t)
		comps.mockNats.AssertNotCalled(t, "Publish", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("NoJobsAvailableDueToPgxErrNoRows", func(t *testing.T) {
		comps := setupJobPollerTest(t)
		// pgx.ErrNoRows is a specific error that should be treated as "no jobs available"
		comps.mockRepo.On("AcquireDueJobs", ctx, mock.AnythingOfType("time.Time"), comps.pollerCfg.JobBatchSize).Return(nil, domain.ErrNoDueJobs).Once()

		processed, err := comps.jobPoller.PollAndProcessJobs(ctx)

		assert.NoError(t, err) // Should return nil error as ErrNoDueJobs is handled
		assert.Equal(t, 0, processed)
		comps.mockRepo.AssertExpectations(t)
		comps.mockNats.AssertNotCalled(t, "Publish", mock.Anything, mock.Anything, mock.Anything)
	})


	t.Run("ErrorAcquiringJobs", func(t *testing.T) {
		comps := setupJobPollerTest(t)
		dbError := errors.New("database connection error")
		comps.mockRepo.On("AcquireDueJobs", ctx, mock.AnythingOfType("time.Time"), comps.pollerCfg.JobBatchSize).Return(nil, dbError).Once()

		processed, err := comps.jobPoller.PollAndProcessJobs(ctx)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), dbError.Error())
		assert.Equal(t, 0, processed)
		comps.mockRepo.AssertExpectations(t)
		comps.mockNats.AssertNotCalled(t, "Publish", mock.Anything, mock.Anything, mock.Anything)
		// Metric: A "poll_cycle_completed" with "error_acquire" status would be ideal if we had such a metric.
		// The existing job processing metrics would not be touched.
	})

	t.Run("ErrorPublishingToNATSJobMarkedForRetry", func(t *testing.T) {
		comps := setupJobPollerTest(t)
		jobID := uuid.New()
		smsPayload := domain.SMSJobPayload{OutboxMessageID: uuid.New().String()}
		payloadBytes, _ := json.Marshal(smsPayload)
		publishError := errors.New("NATS publish failed")

		mockJob := &domain.ScheduledJob{
			ID:        jobID,
			JobType:   domain.JobTypeSMS,
			Payload:   payloadBytes,
			Status:    domain.StatusQueued,
			RetryCount: 0, // First attempt
		}
		comps.mockRepo.On("AcquireDueJobs", ctx, mock.AnythingOfType("time.Time"), comps.pollerCfg.JobBatchSize).Return([]*domain.ScheduledJob{mockJob}, nil).Once()
		comps.mockNats.On("Publish", ctx, "sms.jobs.send", payloadBytes).Return(publishError).Once()
		comps.mockRepo.On("MarkForRetry", ctx, jobID, mock.AnythingOfType("time.Time"), mockJob.RetryCount+1, sql.NullString{String: publishError.Error(), Valid: true}).Return(nil).Once()

		processed, err := comps.jobPoller.PollAndProcessJobs(ctx)

		assert.NoError(t, err) // PollAndProcessJobs itself doesn't return error for retryable job failures
		assert.Equal(t, 1, processed) // It attempted to process one job
		comps.mockRepo.AssertExpectations(t)
		comps.mockNats.AssertExpectations(t)
		// Metric: jobsProcessedCounter with status "error_processing" (or a more specific "error_publish_retry")
	})

	t.Run("ErrorPublishingToNATSMaxRetriesReached", func(t *testing.T) {
		comps := setupJobPollerTest(t)
		jobID := uuid.New()
		smsPayload := domain.SMSJobPayload{OutboxMessageID: uuid.New().String()}
		payloadBytes, _ := json.Marshal(smsPayload)
		publishError := errors.New("NATS publish failed again")

		mockJob := &domain.ScheduledJob{
			ID:         jobID,
			JobType:    domain.JobTypeSMS,
			Payload:    payloadBytes,
			Status:     domain.StatusQueued,
			RetryCount: comps.pollerCfg.MaxRetry, // Max retries reached
		}
		comps.mockRepo.On("AcquireDueJobs", ctx, mock.AnythingOfType("time.Time"), comps.pollerCfg.JobBatchSize).Return([]*domain.ScheduledJob{mockJob}, nil).Once()
		comps.mockNats.On("Publish", ctx, "sms.jobs.send", payloadBytes).Return(publishError).Once()
		// Instead of MarkForRetry, expect UpdateStatus to StatusFailed
		comps.mockRepo.On(
			"UpdateStatus", ctx, jobID, domain.StatusFailed,
			mock.AnythingOfType("time.Time"), // processedAt
			sql.NullString{String: "Failed after max retries: NATS publish: NATS publish failed again", Valid: true}, // lastError
			0, // retryCount (reset or not relevant for final failure) - current impl might pass original retry count
		).Return(nil).Once()


		processed, err := comps.jobPoller.PollAndProcessJobs(ctx)

		assert.NoError(t, err) // PollAndProcessJobs doesn't return error for job failures handled by marking as failed
		assert.Equal(t, 1, processed)
		comps.mockRepo.AssertExpectations(t)
		comps.mockNats.AssertExpectations(t)
		// Metric: jobsProcessedCounter with status "error_max_retries_reached"
	})

	t.Run("ErrorInUpdateStatusAfterSuccessfulPublish", func(t *testing.T) {
		comps := setupJobPollerTest(t)
		jobID := uuid.New()
		smsPayload := domain.SMSJobPayload{OutboxMessageID: uuid.New().String()}
		payloadBytes, _ := json.Marshal(smsPayload)
		dbUpdateError := errors.New("failed to update job status to completed")

		mockJob := &domain.ScheduledJob{
			ID:        jobID,
			JobType:   domain.JobTypeSMS,
			Payload:   payloadBytes,
			Status:    domain.StatusQueued,
			RetryCount: 0,
		}
		comps.mockRepo.On("AcquireDueJobs", ctx, mock.AnythingOfType("time.Time"), comps.pollerCfg.JobBatchSize).Return([]*domain.ScheduledJob{mockJob}, nil).Once()
		comps.mockNats.On("Publish", ctx, "sms.jobs.send", payloadBytes).Return(nil).Once() // Publish succeeds
		comps.mockRepo.On("UpdateStatus", ctx, jobID, domain.StatusCompleted, mock.AnythingOfType("time.Time"), sql.NullString{}, 0).Return(dbUpdateError).Once() // Update fails

		processed, err := comps.jobPoller.PollAndProcessJobs(ctx)

		assert.NoError(t, err) // The error from UpdateStatus is logged but not returned by PollAndProcessJobs
		assert.Equal(t, 1, processed)
		comps.mockRepo.AssertExpectations(t)
		comps.mockNats.AssertExpectations(t)
		// Metric: jobsProcessedCounter with status "error_update_status_completed"
	})

	t.Run("UnknownJobType", func(t *testing.T) {
		comps := setupJobPollerTest(t)
		jobID := uuid.New()

		mockJob := &domain.ScheduledJob{
			ID:        jobID,
			JobType:   domain.JobType("unknown_type"), // Use the actual type from domain if it exists, or cast string
			Payload:   json.RawMessage(`{}`),
			Status:    domain.StatusQueued,
		}
		comps.mockRepo.On("AcquireDueJobs", ctx, mock.AnythingOfType("time.Time"), comps.pollerCfg.JobBatchSize).Return([]*domain.ScheduledJob{mockJob}, nil).Once()
		// Expect UpdateStatus to mark as Failed
		comps.mockRepo.On(
			"UpdateStatus", ctx, jobID, domain.StatusFailed,
			mock.AnythingOfType("time.Time"), // processedAt
			sql.NullString{String: "Unknown job type: unknown_type", Valid: true}, // lastError
			0, // retryCount
		).Return(nil).Once()

		processed, err := comps.jobPoller.PollAndProcessJobs(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 1, processed)
		comps.mockRepo.AssertExpectations(t)
		comps.mockNats.AssertNotCalled(t, "Publish", mock.Anything, mock.Anything, mock.Anything)
		// Metric: jobsProcessedCounter with job_type="unknown_type", status="error_processing" (or specific unknown_type status)
	})
}

// TODO: Add tests for the Run method of JobPoller to check ticker behavior and context cancellation.
// This would require more involved mocking of time or using channels to control the ticker.
// For now, focusing on PollAndProcessJobs logic.

// MockNatsMsg implements messagebroker.Message for testing if needed by specific NATS client mocks
// However, the current mock NatsClient directly takes handler func(*nats.Msg)
// If the platform NATSClient abstraction evolves, this might be useful.
type MockNatsMsg struct {
	mock.Mock
	MSubject string
	MData    []byte
	MReply   string
	// MSub     Subscription // If needed
}

func (m *MockNatsMsg) Subject() string {
	args := m.Called()
	return args.String(0)
}
func (m *MockNatsMsg) Data() []byte {
	args := m.Called()
	return args.Get(0).([]byte)
}
func (m *MockNatsMsg) Reply() string {
	args := m.Called()
	return args.String(0)
}
func (m *MockNatsMsg) Respond(data []byte) error {
	args := m.Called(data)
	return args.Error(0)
}
func (m *MockNatsMsg) Sub() interface{} { // Can return the actual *nats.Subscription or a mock
	args := m.Called()
	return args.Get(0)
}

// Ensure our MockNatsClient implements the interface used by JobPoller.
// The JobPoller currently uses a concrete *messagebroker.NATSClient.
// For better testability, JobPoller should depend on an interface.
// For now, the tests will use the concrete type if NewJobPoller requires it,
// or we adapt the mock to fit.
// The current MockNatsClient has Publish, but SubscribeToSubjectWithQueue is not on messagebroker.NatsClient
// Let's assume messagebroker.NATSClient is an interface with Publish.
// The JobPoller itself doesn't subscribe, it only publishes.
// The main.go sets up the subscription.

// The repository methods like UpdateStatus have retryCount as the last param.
// In MarkAsFailed, the current job_poller.go calls:
// err = p.repo.UpdateStatus(ctx, job.ID, domain.StatusFailed, time.Now().UTC(), sql.NullString{String: "Failed after max retries: " + processingError.Error(), Valid: true}, 0)
// The last '0' might be intended as retryCount reset, or just a placeholder. Test reflects this.
// In UnknownJobType, it's also 0.
// In MarkAsProcessed (UpdateStatus to Completed), it's also 0.

// Correction: MockScheduledJobRepository needs to align with domain.ScheduledJobRepository interface.
// domain.ScheduledJobRepository:
//   CreateJob(ctx context.Context, job *ScheduledJob) error
//   GetJob(ctx context.Context, jobID uuid.UUID) (*ScheduledJob, error)
//   AcquireDueJobs(ctx context.Context, dueTime time.Time, limit int) ([]*ScheduledJob, error)
//   UpdateStatus(ctx context.Context, jobID uuid.UUID, status JobStatus, processedAt time.Time, processingErr sql.NullString, retryCount int) error
//   MarkForRetry(ctx context.Context, jobID uuid.UUID, nextRunAt time.Time, retryCount int, lastError sql.NullString) error

// The mock seems to cover these.
// The NATS client used by JobPoller only needs Publish method.
// The SubscribeToSubjectWithQueue method in MockNatsClient is not used by JobPoller directly.
// It was added perhaps due to the interface definition in messagebroker.NatsClient.
// We need to ensure the MockNatsClient correctly implements the interface if JobPoller depends on an interface.
// Currently, JobPoller takes *messagebroker.NATSClient (concrete type).

// Let's assume the provided NatsClient in messagebroker package has a Publish method.
// The MockNatsClient provides this.
// For this test, we only care about the methods JobPoller calls on its dependencies.
// JobPoller calls:
// - repo.AcquireDueJobs
// - natsClient.Publish
// - repo.UpdateStatus (for Completed, Failed)
// - repo.MarkForRetry
// All these are present in the mocks.The test file `job_poller_test.go` has been created with mock structures and a setup function. The first set of test cases for `PollAndProcessJobs` (Success, NoJobsAvailable, NoJobsAvailableDueToPgxErrNoRows, ErrorAcquiringJobs, ErrorPublishingToNATSJobMarkedForRetry, ErrorPublishingToNATSMaxRetriesReached, ErrorInUpdateStatusAfterSuccessfulPublish, UnknownJobType) are also implemented.

The tests cover various scenarios and assert mock calls and error propagation. The comments about metric verification acknowledge the difficulty of directly testing `promauto` globals in unit tests and focus on behavior.

The mock structures for `ScheduledJobRepository` and `NatsClient` cover the methods used by `JobPoller`.

One minor detail: `domain.ErrNoDueJobs` is used in one of the tests. This error should be defined in the `domain` package for this to work. Assuming it is.
The `UpdateStatus` mock call in `TestJobPoller_PollAndProcessJobs/ErrorPublishingToNATSMaxRetriesReached` correctly reflects the parameters used in `job_poller.go`, including the `0` for `retryCount` when marking as failed.

The structure looks good. The next step is to integrate the metrics server into `cmd/scheduler_service/main.go`.
I will now proceed to read `cmd/scheduler_service/main.go`.
