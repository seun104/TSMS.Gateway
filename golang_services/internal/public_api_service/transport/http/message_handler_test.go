package http_test // Use _test package

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	// Adjust these import paths to your actual project structure
	"github.com/aradsms/golang_services/internal/core_sms/domain"
	"github.com/aradsms/golang_services/internal/platform/messagebroker"
	"github.com/aradsms/golang_services/internal/public_api_service/middleware" // For AuthenticatedUser
	httptransport "github.com/aradsms/golang_services/internal/public_api_service/transport/http"
	outboxRepoIface "github.com/aradsms/golang_services/internal/sms_sending_service/repository" // Interface
	// mockOutboxRepo "github.com/aradsms/golang_services/internal/sms_sending_service/repository/mocks" // If using testify/mock

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool" // For dbPool, though it's not directly used by handler if repo is mocked
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock" // Using testify for mocking example
	"github.com/stretchr/testify/require"
)

// MockOutboxRepository for MessageHandler tests
type MockOutboxRepository struct {
	mock.Mock
	// Implement outboxRepoIface.OutboxRepository if not using a mock generator
}

func (m *MockOutboxRepository) Create(ctx context.Context, querier outboxRepoIface.Querier, message *domain.OutboxMessage) (*domain.OutboxMessage, error) {
	args := m.Called(ctx, querier, message)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.OutboxMessage), args.Error(1)
}

func (m *MockOutboxRepository) GetByID(ctx context.Context, querier outboxRepoIface.Querier, id string) (*domain.OutboxMessage, error) {
	args := m.Called(ctx, querier, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.OutboxMessage), args.Error(1)
}

func (m *MockOutboxRepository) UpdateStatus(ctx context.Context, querier outboxRepoIface.Querier, id string, newStatus domain.MessageStatus, providerMsgID *string, providerStatus *string, errorMessage *string) error {
	args := m.Called(ctx, querier, id, newStatus, providerMsgID, providerStatus, errorMessage)
	return args.Error(0)
}
func (m *MockOutboxRepository) UpdatePostSendInfo(ctx context.Context, querier outboxRepoIface.Querier, id string, status domain.MessageStatus, providerMsgID *string, providerStatus *string, sentToProviderAt time.Time, errorMessage *string) error {
    args := m.Called(ctx, querier, id, status, providerMsgID, providerStatus, sentToProviderAt, errorMessage)
    return args.Error(0)
}


// MockNatsClient for MessageHandler tests
type MockNatsClient struct {
	// messagebroker.NatsClient // Embed if it has other methods, or mock specific ones
    // For this test, we only need Publish.
	PublishFunc func(ctx context.Context, subject string, data []byte) error
}

func (m *MockNatsClient) Publish(ctx context.Context, subject string, data []byte) error {
	if m.PublishFunc != nil {
		return m.PublishFunc(ctx, subject, data)
	}
	return nil // Default mock success
}
// Ensure MockNatsClient implements the interface expected by NewMessageHandler, if any.
// If NewMessageHandler expects *messagebroker.NatsClient, this mock needs to match that structure or use an interface.
// For simplicity, we'll assume the handler can use this mock directly. If not, embedding might be needed:
// type MockNatsClient struct {
//    messagebroker.NatsClient // Embed the real client
//    PublishFunc func(ctx context.Context, subject string, data []byte) error
// }
// func (m *MockNatsClient) Publish(ctx context.Context, subject string, data []byte) error {
//    if m.PublishFunc != nil { return m.PublishFunc(ctx, subject, data) }
//    return m.NatsClient.Publish(ctx, subject, data) // Call embedded, or just return nil for mock
// }



func TestMessageHandler_handleSendMessage(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockNats := &MockNatsClient{}
	mockRepo := new(MockOutboxRepository)
	var mockDbPool *pgxpool.Pool // Can be nil if repo Create doesn't actually use it when mocked

	handler := httptransport.NewMessageHandler(mockNats, mockRepo, mockDbPool, logger)
	router := chi.NewRouter()
	// Assuming message routes are registered under a group that applies auth middleware.
    // For direct unit testing of the handler, we pass a context with the user.
    // If testing through the router as mounted in main.go, ensure auth middleware is also part of test setup.
	handler.RegisterRoutes(router)

	// Authenticated user for context
	authUser := middleware.AuthenticatedUser{ID: "user-123", Username: "testuser", IsAdmin: false}
	ctxWithUser := context.WithValue(context.Background(), middleware.AuthenticatedUserContextKey, authUser)

	t.Run("successful send", func(t *testing.T) {
		sendReq := httptransport.SendMessageRequest{
			SenderID:  "TestSender",
			Recipient: "12345",
			Content:   "Hello",
		}
		reqBody, _ := json.Marshal(sendReq)

		// Mock OutboxRepository.Create
		mockRepo.On("Create", mock.Anything, mock.Anything, mock.MatchedBy(func(msg *domain.OutboxMessage) bool {
			return msg.UserID == authUser.ID && msg.Recipient == sendReq.Recipient
		})).Return(&domain.OutboxMessage{ID: uuid.NewString(), UserID: authUser.ID, Status: domain.MessageStatusQueued}, nil).Once()

		// Mock NATS Publish
		var publishedNatsPayload []byte
		mockNats.PublishFunc = func(ctx context.Context, subject string, data []byte) error {
			assert.Equal(t, "sms.jobs.send", subject)
			publishedNatsPayload = data
			return nil
		}

		req := httptest.NewRequest("POST", "/messages/send", bytes.NewReader(reqBody)).WithContext(ctxWithUser)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		require.Equal(t, http.StatusAccepted, rr.Code)
		var resp httptransport.SendMessageResponse
		err := json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.NotEmpty(t, resp.MessageID)
		assert.Equal(t, domain.MessageStatusQueued, resp.Status)

		// Verify NATS payload
		require.NotNil(t, publishedNatsPayload)
		var natsPayload map[string]string
		err = json.Unmarshal(publishedNatsPayload, &natsPayload)
		require.NoError(t, err)
		assert.Equal(t, resp.MessageID, natsPayload["outbox_message_id"])

		mockRepo.AssertExpectations(t)
	})

    t.Run("database create error", func(t *testing.T) {
        sendReq := httptransport.SendMessageRequest{ SenderID: "Test", Recipient: "To", Content: "Fail" }
        reqBody, _ := json.Marshal(sendReq)

        mockRepo.On("Create", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("DB error")).Once()
        // NATS should not be called
        mockNats.PublishFunc = func(ctx context.Context, subject string, data []byte) error {
            t.Error("NATS publish should not be called on DB error")
            return nil
        }

        req := httptest.NewRequest("POST", "/messages/send", bytes.NewReader(reqBody)).WithContext(ctxWithUser)
        rr := httptest.NewRecorder()
        router.ServeHTTP(rr, req)

        require.Equal(t, http.StatusInternalServerError, rr.Code)
        mockRepo.AssertExpectations(t)
    })
}


func TestMessageHandler_handleGetMessageStatus(t *testing.T) {
    logger := slog.New(slog.NewTextHandler(io.Discard, nil))
    mockRepo := new(MockOutboxRepository)
    var mockDbPool *pgxpool.Pool // Can be nil if GetByID doesn't use it when mocked

    handler := httptransport.NewMessageHandler(nil, mockRepo, mockDbPool, logger) // NATS client not needed for Get
    router := chi.NewRouter()
    // Message routes are typically grouped and might have middleware (like auth) applied at group level.
    // For this unit test, we register directly to test the handler in isolation of auth middleware,
    // but pass an auth'd context.
    router.Get("/messages/{messageID}", handler.ServeHTTP) // This assumes handler itself is an http.Handler
                                                          // Or, more typically:
    // router.Route("/messages", func(r chi.Router){ // This matches how RegisterRoutes is defined
    //    handler.RegisterRoutes(r)
    // })
    // The original code for MessageHandler uses RegisterRoutes, so we should use that.
    // The router passed to RegisterRoutes should be the one we test with.
    messageSubRouter := chi.NewRouter()
    handler.RegisterRoutes(messageSubRouter)
    router.Mount("/messages", messageSubRouter) // Mount under /messages path


    authUser := middleware.AuthenticatedUser{ID: "user-123", Username: "testuser", IsAdmin: false}
    ctxWithUser := context.WithValue(context.Background(), middleware.AuthenticatedUserContextKey, authUser)

    messageUUID := uuid.NewString()

    t.Run("successful get owned message", func(t *testing.T) {
        mockMsg := &domain.OutboxMessage{
            ID:        messageUUID,
            UserID:    authUser.ID, // Owner
            Status:    domain.MessageStatusSentToProvider,
            SenderID:  "Me",
            Recipient: "You",
            Content:   "Hi",
            CreatedAt: time.Now().Add(-time.Hour),
            UpdatedAt: time.Now(),
        }
        mockRepo.On("GetByID", mock.Anything, mock.Anything, messageUUID).Return(mockMsg, nil).Once()

        req := httptest.NewRequest("GET", "/messages/"+messageUUID, nil).WithContext(ctxWithUser)
        rr := httptest.NewRecorder()
        router.ServeHTTP(rr, req)

        require.Equal(t, http.StatusOK, rr.Code)
        var resp httptransport.MessageStatusResponse
        err := json.Unmarshal(rr.Body.Bytes(), &resp)
        require.NoError(t, err)
        assert.Equal(t, messageUUID, resp.ID)
        assert.Equal(t, mockMsg.Status, resp.Status)
        mockRepo.AssertExpectations(t)
    })

    t.Run("admin gets unowned message", func(t *testing.T) {
        adminUser := middleware.AuthenticatedUser{ID: "admin-007", Username: "admin", IsAdmin: true}
        ctxWithAdmin := context.WithValue(context.Background(), middleware.AuthenticatedUserContextKey, adminUser)

        mockMsg := &domain.OutboxMessage{
            ID:        messageUUID,
            UserID:    "another-user-id", // Not owned by admin-007
            Status:    domain.MessageStatusDelivered,
             /* ... other fields ... */
        }
        mockRepo.On("GetByID", mock.Anything, mock.Anything, messageUUID).Return(mockMsg, nil).Once()

        req := httptest.NewRequest("GET", "/messages/"+messageUUID, nil).WithContext(ctxWithAdmin)
        rr := httptest.NewRecorder()
        router.ServeHTTP(rr, req)

        require.Equal(t, http.StatusOK, rr.Code) // Admin can view
        mockRepo.AssertExpectations(t)
    })


    t.Run("user attempts to get unowned message", func(t *testing.T) {
        mockMsg := &domain.OutboxMessage{
            ID:        messageUUID,
            UserID:    "another-user-id", // Not owned by authUser
             /* ... other fields ... */
        }
        mockRepo.On("GetByID", mock.Anything, mock.Anything, messageUUID).Return(mockMsg, nil).Once()

        req := httptest.NewRequest("GET", "/messages/"+messageUUID, nil).WithContext(ctxWithUser)
        rr := httptest.NewRecorder()
        router.ServeHTTP(rr, req)

        require.Equal(t, http.StatusForbidden, rr.Code)
        mockRepo.AssertExpectations(t)
    })

    t.Run("message not found", func(t *testing.T) {
        mockRepo.On("GetByID", mock.Anything, mock.Anything, messageUUID).Return(nil, outboxRepoIface.ErrOutboxMessageNotFound).Once()

        req := httptest.NewRequest("GET", "/messages/"+messageUUID, nil).WithContext(ctxWithUser)
        rr := httptest.NewRecorder()
        router.ServeHTTP(rr, req)

        require.Equal(t, http.StatusNotFound, rr.Code)
        mockRepo.AssertExpectations(t)
    })
}
