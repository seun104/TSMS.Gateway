package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/AradIT/aradsms/golang_services/internal/inbound_processor_service/domain"
	"github.com/AradIT/aradsms/golang_services/internal/platform/messagebroker"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// --- Mocks ---

type MockInboxRepository struct {
	mock.Mock
}

func (m *MockInboxRepository) Create(ctx context.Context, msg *domain.InboxMessage) error {
	args := m.Called(ctx, msg)
	return args.Error(0)
}

func (m *MockInboxRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.InboxMessage, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.InboxMessage), args.Error(1)
}

type MockPrivateNumberRepository struct {
	mock.Mock
}

func (m *MockPrivateNumberRepository) FindByNumber(ctx context.Context, number string) (*domain.PrivateNumber, error) {
	args := m.Called(ctx, number)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.PrivateNumber), args.Error(1)
}
func (m *MockPrivateNumberRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.PrivateNumber, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.PrivateNumber), args.Error(1)
}


type MockNATSClient struct {
	mock.Mock
}

func (m *MockNATSClient) Publish(ctx context.Context, subject string, data []byte) error {
	args := m.Called(ctx, subject, data)
	return args.Error(0)
}

// Subscribe and other methods if needed by tests, for now only Publish for keyword events.
func (m *MockNATSClient) Subscribe(ctx context.Context, subject string, queueGroup string, handler func(msg messagebroker.Message)) (messagebroker.Subscription, error) {
	args := m.Called(ctx, subject, queueGroup, handler)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(messagebroker.Subscription), args.Error(1)
}
func (m *MockNATSClient) Close() {}


// --- Tests ---

func setupProcessorTest(t *testing.T) (*SMSProcessor, *MockInboxRepository, *MockPrivateNumberRepository, *MockNATSClient) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil)) // Discard logs for test clarity
	mockInboxRepo := new(MockInboxRepository)
	mockPrivateNumRepo := new(MockPrivateNumberRepository)
	mockNATS := new(MockNATSClient)

	processor := NewSMSProcessor(mockInboxRepo, mockPrivateNumRepo, mockNATS, logger)
	return processor, mockInboxRepo, mockPrivateNumRepo, mockNATS
}

func TestSMSProcessor_ParseAndActOnKeywords(t *testing.T) {
	processor, _, _, mockNATS := setupProcessorTest(t)

	testCases := []struct {
		name             string
		messageText      string
		expectedKeyword  string
		expectedSubject  string
		expectPublish    bool
		publishError     error
		setupNATSMock    func(keyword, subject string, publishError error)
	}{
		{
			name:            "STOP keyword exact match",
			messageText:     "STOP",
			expectedKeyword: "STOP",
			expectedSubject: "inbound.keyword.stop",
			expectPublish:   true,
			setupNATSMock: func(keyword, subject string, publishError error) {
				mockNATS.On("Publish", context.Background(), subject, mock.MatchedBy(func(data []byte) bool {
					var event KeywordDetectedEvent
					json.Unmarshal(data, &event)
					return event.MatchedKeyword == keyword && event.OriginalText == "STOP"
				})).Return(publishError).Once()
			},
		},
		{
			name:            "HELP keyword case insensitive with extra text",
			messageText:     "help me please",
			expectedKeyword: "HELP",
			expectedSubject: "inbound.keyword.help",
			expectPublish:   true,
			setupNATSMock: func(keyword, subject string, publishError error) {
				mockNATS.On("Publish", context.Background(), subject, mock.MatchedBy(func(data []byte) bool {
					var event KeywordDetectedEvent
					json.Unmarshal(data, &event)
					return event.MatchedKeyword == keyword && event.OriginalText == "help me please"
				})).Return(publishError).Once()
			},
		},
		{
			name:            "No keyword match",
			messageText:     "Hello there",
			expectPublish:   false,
			setupNATSMock:   func(keyword, subject string, publishError error) {},
		},
		{
			name:            "Keyword UNSTOP does not match STOP",
			messageText:     "UNSTOP",
			expectPublish:   false,
			setupNATSMock:   func(keyword, subject string, publishError error) {},
		},
		{
			name:            "INFO keyword with trailing space",
			messageText:     "INFO ",
			expectedKeyword: "INFO",
			expectedSubject: "inbound.keyword.info",
			expectPublish:   true,
			setupNATSMock: func(keyword, subject string, publishError error) {
				mockNATS.On("Publish", context.Background(), subject, mock.MatchedBy(func(data []byte) bool {
					var event KeywordDetectedEvent
					json.Unmarshal(data, &event)
					return event.MatchedKeyword == keyword && event.OriginalText == "INFO "
				})).Return(publishError).Once()
			},
		},
		{
			name:            "NATS publish failure for STOP keyword",
			messageText:     "STOP",
			expectedKeyword: "STOP",
			expectedSubject: "inbound.keyword.stop",
			expectPublish:   true,
			publishError:    errors.New("nats publish failed"),
			setupNATSMock: func(keyword, subject string, publishError error) {
				mockNATS.On("Publish", context.Background(), subject, mock.AnythingOfType("[]uint8")).Return(publishError).Once()
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset mock for each test case if On() was called without Once() or Times() in setup
			// For this structure, mockNATS is new per test case effectively due to setupNATSMock
			currentMockNATS := new(MockNATSClient) // Use a fresh mock for each sub-test
			processor.natsClient = currentMockNATS // Inject fresh mock

			if tc.expectPublish {
				// Setup the expectation for the current test case
				tc.setupNATSMock(tc.expectedKeyword, tc.expectedSubject, tc.publishError)
			}

			msg := &domain.InboxMessage{
				ID:                  uuid.New(),
				Text:                tc.messageText,
				ReceivedByGatewayAt: time.Now(),
				// Other fields can be zero/default for this specific test focus
			}
			processor.parseAndActOnKeywords(context.Background(), msg)

			currentMockNATS.AssertExpectations(t)
		})
	}
}

// TestProcessMessage_KeywordPath is an integration-style test for ProcessMessage
// focusing on ensuring parseAndActOnKeywords is called.
func TestProcessMessage_KeywordPath(t *testing.T) {
	processor, mockInboxRepo, mockPrivateNumRepo, mockNATS := setupProcessorTest(t)

	event := InboundSMSEvent{
		ProviderName: "test-provider",
		Data: ProviderIncomingSMSRequest{ // Assuming ProviderIncomingSMSRequest is the type for Data
			MessageID: "prov-msg-id-stop",
			From:      "sender-num",
			To:        "private-num-stop",
			Text:      "STOP",
			Timestamp: time.Now(),
		},
	}

	// Mock FindByNumber to return nil (no association) for simplicity for this test
	mockPrivateNumRepo.On("FindByNumber", mock.Anything, event.Data.To).Return(nil, nil).Once()
	// Mock InboxRepository.Create to succeed
	mockInboxRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.InboxMessage")).Return(nil).Once()

	// Expect NATS publish for "STOP" keyword
	expectedSubject := keywordActions["STOP"]
	mockNATS.On("Publish", mock.Anything, expectedSubject, mock.MatchedBy(func(data []byte) bool {
		var kwEvent KeywordDetectedEvent
		err := json.Unmarshal(data, &kwEvent)
		return err == nil && kwEvent.MatchedKeyword == "STOP" && kwEvent.OriginalText == "STOP"
	})).Return(nil).Once()

	err := processor.ProcessMessage(context.Background(), event)
	assert.NoError(t, err)

	mockInboxRepo.AssertExpectations(t)
	mockPrivateNumRepo.AssertExpectations(t)
	mockNATS.AssertExpectations(t)
}


// TODO: Add more comprehensive tests for ProcessMessage if not already covered elsewhere,
// including association logic, error handling from repository, etc.
// The existing tests for ProcessMessage (if any) should be maintained.
// This file primarily adds tests for the keyword parsing aspect.

[filepath]golang_services/internal/inbound_processor_service/app/consumer.go
// Placeholder for consumer.go if InboundSMSEvent is defined there.
// This is just to make the compiler potentially happier if the type is expected from here.
// For the purpose of this subtask, the structure of InboundSMSEvent is assumed.

package app

import "time"

// ProviderIncomingSMSRequest represents the `Data` field in InboundSMSEvent.
// This structure is assumed based on its usage in sms_processor.go.
type ProviderIncomingSMSRequest struct {
	MessageID string    `json:"message_id"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"` // When the provider received it
}

// InboundSMSEvent is the structure of messages consumed from NATS (e.g., "sms.incoming.raw.*").
// This structure is assumed based on its usage in sms_processor.go.
type InboundSMSEvent struct {
	ProviderName string                     `json:"provider_name"`
	Data         ProviderIncomingSMSRequest `json:"data"`
}
