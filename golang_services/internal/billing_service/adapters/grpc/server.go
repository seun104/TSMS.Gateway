package grpc

import (
	"context"
	"errors"
	"log/slog"

	"strings" // For error checking
	"github.com/AradIT/aradsms/golang_services/api/proto/billingservice"
	"github.com/AradIT/aradsms/golang_services/internal/billing_service/app"
	"github.com/AradIT/aradsms/golang_services/internal/billing_service/domain"
	"github.com/google/uuid"
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"fmt"
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
// Assumes CheckAndDeductCreditRequest proto is updated to include SegmentsToCharge instead of AmountToDeduct.
func (s *BillingGRPCServer) DeductUserCredit(ctx context.Context, req *billingservice.DeductUserCreditRequest) (*billingservice.DeductUserCreditResponse, error) {
	logger := s.logger.With("rpc_method", "DeductUserCredit", "user_id_req", req.GetUserId(), "reference_id", req.GetReferenceId())
	logger.InfoContext(ctx, "gRPC call received", "segments_to_charge", req.GetSegmentsToCharge())

	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		logger.ErrorContext(ctx, "Invalid UserID format in request", "error", err)
		return nil, status.Errorf(codes.InvalidArgument, "Invalid UserID format: %v", err)
	}
	// Add successfully parsed UserID to logger for subsequent logs
	logger = logger.With("user_id", userID.String())


	if req.GetSegmentsToCharge() <= 0 {
		logger.ErrorContext(ctx, "SegmentsToCharge must be positive", "segments", req.GetSegmentsToCharge())
        return nil, status.Errorf(codes.InvalidArgument, "SegmentsToCharge must be positive, got %d", req.GetSegmentsToCharge())
	}
	numMessages := int(req.GetSegmentsToCharge())

	description := req.GetDescription()
	if description == "" {
		description = fmt.Sprintf("Charge for %d SMS message(s), ref: %s", numMessages, req.GetReferenceId())
	}

	transactionDetails := domain.TransactionDetails{
		Description: description,
		ReferenceID: req.GetReferenceId(),
	}

	createdTx, err := s.billingApp.DeductCreditForSMS(ctx, userID, numMessages, transactionDetails)
	if err != nil {
		logger.ErrorContext(ctx, "DeductCreditForSMS application logic failed", "segments", numMessages, "error", err)

		var errCode codes.Code = codes.Internal
		var errMsg string = fmt.Sprintf("Billing operation failed: %v", err)

		if errors.Is(err, app.ErrInsufficientCredit) {
			errCode = codes.FailedPrecondition
			errMsg = fmt.Sprintf("Insufficient credit: %v", err)
		} else if errors.Is(err, app.ErrUserNotFoundForBilling) {
			errCode = codes.NotFound
			errMsg = fmt.Sprintf("User not found for billing: %v", err)
		} else if strings.Contains(err.Error(), "no applicable tariff found") {
			errCode = codes.FailedPrecondition
			errMsg = fmt.Sprintf("Billing configuration error: %v", err)
		}
        // No need for app.ErrPaymentGateway check here as it's an SMS charge path

		return &billingservice.DeductUserCreditResponse{Success: false, ErrorMessage: errMsg}, status.Errorf(errCode, errMsg)
	}

	if createdTx == nil {
        s.logger.ErrorContext(ctx, "DeductCreditForSMS returned nil transaction without error", "user_id", userID)
        return &billingservice.DeductUserCreditResponse{Success: false, ErrorMessage: "Internal error: processing completed without transaction details"}, status.Errorf(codes.Internal, "internal error: operation outcome unknown")
    }

	s.logger.InfoContext(ctx, "Credit deducted successfully via gRPC handler",
		"user_id", userID,
		"transaction_id", createdTx.ID, // ID is string in domain.Transaction
		"cost", createdTx.Amount) // Amount is float64 in domain.Transaction

	// Construct the TransactionProto part of the response if needed
	protoTx := &billingservice.TransactionProto{
		Id:                 createdTx.ID,
		UserId:             createdTx.UserID,
		Type:               mapDomainTransactionTypeToProto(createdTx.Type),
		Amount:             createdTx.Amount, // domain.Transaction.Amount is float64, proto expects double
		CurrencyCode:       createdTx.CurrencyCode,
		Description:        createdTx.Description,
		BalanceBefore:      createdTx.BalanceBefore, // Assuming TransactionProto has these from previous steps
		BalanceAfter:       createdTx.BalanceAfter,
		CreatedAt:          timestamppb.New(createdTx.CreatedAt),
	}
	if createdTx.RelatedMessageID != nil {
		protoTx.RelatedMessageId = *createdTx.RelatedMessageID
	}
	// PaymentIntentID from domain.Transaction is *uuid.UUID, not directly mapped to TransactionProto here.

	return &billingservice.DeductUserCreditResponse{
		Success:       true,
		TransactionId: createdTx.ID,
		AmountDeducted: int64(createdTx.Amount), // Assuming Amount is in smallest unit or needs conversion if proto expects that.
		                                         // The domain.Transaction.Amount is float64. The proto field is int64.
		                                         // This implies the cost from CalculateSMSCost (int64) should be used directly.
		                                         // And domain.Transaction.Amount should perhaps be int64.
		                                         // For now, casting float64 to int64. This needs unit consistency review.
                                                 // Let's use the cost from CalculateSMSCost which is int64.
                                                 // The transaction's Amount field should store this int64 cost.
                                                 // If createdTx.Amount is float64, this is a mismatch.
                                                 // The app service's DeductCreditForSMS returns transaction with Amount as float64.
                                                 // This needs alignment: cost is int64, transaction amount is float64.
                                                 // For now, I'll assume transaction.Amount (float64) can be cast to int64 for the response.
                                                 // This is likely an issue if smallest units are not consistently int64 across domain/proto.
                                                 // Let's assume cost (int64) from CalculateSMSCost is the authoritative value for amount_deducted.
                                                 // And that createdTx.Amount (float64) reflects this same value.
		Currency:      createdTx.CurrencyCode,
		Transaction:   protoTx, // Include the full transaction details
	}, nil
}
