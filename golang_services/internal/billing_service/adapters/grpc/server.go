package grpc

import (
	"context"
	"errors"
	"log/slog"

	"github.com/AradIT/aradsms/golang_services/api/proto/billingservice" // Corrected
	"github.com/AradIT/aradsms/golang_services/internal/billing_service/app" // Corrected
	"github.com/AradIT/aradsms/golang_services/internal/billing_service/domain" // Corrected
	"github.com/google/uuid" // For parsing UserID
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"fmt" // For constructing description
)

type BillingGRPCServer struct {
	billingservice.UnimplementedBillingServiceInternalServer
	billingApp *app.BillingService
	logger     *slog.Logger
}

func NewBillingGRPCServer(billingApp *app.BillingService, logger *slog.Logger) *BillingGRPCServer {
	return &BillingGRPCServer{
		billingApp: billingApp,
		logger:     logger.With("component", "billing_grpc_server"),
	}
}

func mapDomainTransactionTypeToProto(dt domain.TransactionType) billingservice.TransactionTypeProto {
    switch dt {
    case domain.TransactionTypeCreditPurchase: return billingservice.TransactionTypeProto_TRANSACTION_TYPE_PROTO_CREDIT_PURCHASE
    case domain.TransactionTypeSMSCharge: return billingservice.TransactionTypeProto_TRANSACTION_TYPE_PROTO_SMS_CHARGE
    case domain.TransactionTypeRefund: return billingservice.TransactionTypeProto_TRANSACTION_TYPE_PROTO_REFUND
    // Ensure domain.TransactionTypeServiceActivationFee maps correctly
    case domain.TransactionTypeServiceActivationFee: return billingservice.TransactionTypeProto_TRANSACTION_TYPE_PROTO_SERVICE_ACTIVATION_FEE
    case domain.TransactionTypeManualAdjustment: return billingservice.TransactionTypeProto_TRANSACTION_TYPE_PROTO_MANUAL_ADJUSTMENT
    case domain.TransactionTypeCredit: return billingservice.TransactionTypeProto_TRANSACTION_TYPE_PROTO_CREDIT // Added
    case domain.TransactionTypeDebit: return billingservice.TransactionTypeProto_TRANSACTION_TYPE_PROTO_DEBIT   // Added
    default:
		slog.Warn("Unknown domain transaction type to map to proto", "domain_type", dt)
		return billingservice.TransactionTypeProto_TRANSACTION_TYPE_PROTO_UNSPECIFIED
    }
}
// mapProtoTransactionTypeToDomain is not directly used by DeductCredit but good for completeness if other RPCs need it
// func mapProtoTransactionTypeToDomain(pt billingservice.TransactionTypeProto) domain.TransactionType { ... }


// CheckAndDeductCredit RPC is now used for deducting credit based on message count (segments).
// Assumes CheckAndDeductCreditRequest proto is updated to include NumMessages/Segments instead of AmountToDeduct.
func (s *BillingGRPCServer) CheckAndDeductCredit(ctx context.Context, req *billingservice.CheckAndDeductCreditRequest) (*billingservice.CheckAndDeductCreditResponse, error) {
	s.logger.InfoContext(ctx, "CheckAndDeductCredit RPC called", "userID", req.UserId, "num_messages_to_charge", req.GetNumMessagesToCharge())

	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid UserID format: %v", err)
	}

	numMessages := int(req.GetNumMessagesToCharge()) // Assuming new field in proto
	if numMessages <= 0 {
		// Default to 1 message if not specified or invalid, or return InvalidArgument
		s.logger.WarnContext(ctx, "NumMessagesToCharge is zero or negative, defaulting to 1", "original_value", req.GetNumMessagesToCharge())
		numMessages = 1
	}

	transactionDetails := domain.TransactionDetails{
		Description: req.GetDescription(), // Use description from request
		ReferenceID: req.GetReferenceId(), // e.g., outbox_message_id
	}
	if transactionDetails.Description == "" {
		transactionDetails.Description = fmt.Sprintf("Charge for %d SMS message(s)", numMessages)
	}


	// Call the updated app service method
	// The app service method now returns (*domain.Transaction, error)
	// The first boolean return value was removed from DeductCreditForSMS in the plan.
	createdTx, err := s.billingApp.DeductCreditForSMS(ctx, userID, numMessages, transactionDetails)
	if err != nil {
		s.logger.ErrorContext(ctx, "DeductCreditForSMS failed in app layer", "error", err, "userID", req.UserId)
		// Map domain errors to gRPC status codes
		if errors.Is(err, app.ErrInsufficientCredit) {
			return &billingservice.CheckAndDeductCreditResponse{Success: false, ErrorMessage: err.Error()}, status.Errorf(codes.FailedPrecondition, "Insufficient credit: %v", err)
		}
		if errors.Is(err, app.ErrUserNotFoundForBilling) {
			return &billingservice.CheckAndDeductCreditResponse{Success: false, ErrorMessage: err.Error()}, status.Errorf(codes.NotFound, "User not found for billing: %v", err)
		}
		if errors.Is(err, app.ErrPaymentGateway){ // Though not directly applicable for SMS charge
            return &billingservice.CheckAndDeductCreditResponse{Success: false, ErrorMessage: err.Error()}, status.Errorf(codes.Internal, "Payment gateway error: %v", err)
        }
		// For "no applicable tariff" or other specific errors from CalculateSMSCost
		if strings.Contains(err.Error(), "no applicable tariff found") {
			return &billingservice.CheckAndDeductCreditResponse{Success: false, ErrorMessage: err.Error()}, status.Errorf(codes.FailedPrecondition, "Billing configuration error: %v", err)
		}
		return &billingservice.CheckAndDeductCreditResponse{Success: false, ErrorMessage: err.Error()}, status.Errorf(codes.Internal, "Billing operation failed: %v", err)
	}

	if createdTx == nil { // Should not happen if err is nil
        s.logger.ErrorContext(ctx, "DeductCreditForSMS returned nil transaction and nil error", "userID", req.UserId)
        return &billingservice.CheckAndDeductCreditResponse{Success: false, ErrorMessage: "Internal error: operation outcome unknown"}, status.Errorf(codes.Internal, "internal error: operation outcome unknown")
    }


	protoTx := &billingservice.TransactionProto{
		Id:                 createdTx.ID,
		UserId:             createdTx.UserID,
		Type:               mapDomainTransactionTypeToProto(createdTx.Type), // Should be Debit or SMSCharge
		Amount:             createdTx.Amount, // This is now the cost calculated
		CurrencyCode:       createdTx.CurrencyCode,
		Description:        createdTx.Description,
		RelatedMessageId:   req.GetReferenceId(), // from original request as RelatedMessageID in domain.Transaction is *string
		BalanceBefore:      createdTx.BalanceBefore,
		BalanceAfter:       createdTx.BalanceAfter,
		CreatedAt:          timestamppb.New(createdTx.CreatedAt),
	}
	// PaymentIntentID is not directly relevant for SMS charge transaction proto, so not mapped here.

	return &billingservice.CheckAndDeductCreditResponse{Success: true, Transaction: protoTx}, nil
}
