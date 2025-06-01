package grpc

import (
	"context"
	"errors"
	"log/slog"

	"github.com/aradsms/golang_services/api/proto/billingservice" // Adjust
	"github.com/aradsms/golang_services/internal/billing_service/app"
	"github.com/aradsms/golang_services/internal/billing_service/domain"
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb" // For timestamp conversion
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
    case domain.TransactionTypeServiceActivation: return billingservice.TransactionTypeProto_TRANSACTION_TYPE_PROTO_SERVICE_ACTIVATION_FEE
    case domain.TransactionTypeManualAdjustment: return billingservice.TransactionTypeProto_TRANSACTION_TYPE_PROTO_MANUAL_ADJUSTMENT
    default: return billingservice.TransactionTypeProto_TRANSACTION_TYPE_PROTO_UNSPECIFIED
    }
}
func mapProtoTransactionTypeToDomain(pt billingservice.TransactionTypeProto) domain.TransactionType {
    switch pt {
    case billingservice.TransactionTypeProto_TRANSACTION_TYPE_PROTO_CREDIT_PURCHASE: return domain.TransactionTypeCreditPurchase
    case billingservice.TransactionTypeProto_TRANSACTION_TYPE_PROTO_SMS_CHARGE: return domain.TransactionTypeSMSCharge
    case billingservice.TransactionTypeProto_TRANSACTION_TYPE_PROTO_REFUND: return domain.TransactionTypeRefund
    case billingservice.TransactionTypeProto_TRANSACTION_TYPE_PROTO_SERVICE_ACTIVATION_FEE: return domain.TransactionTypeServiceActivation
    case billingservice.TransactionTypeProto_TRANSACTION_TYPE_PROTO_MANUAL_ADJUSTMENT: return domain.TransactionTypeManualAdjustment
    default: return "" // Or handle error
    }
}


func (s *BillingGRPCServer) CheckAndDeductCredit(ctx context.Context, req *billingservice.CheckAndDeductCreditRequest) (*billingservice.CheckAndDeductCreditResponse, error) {
	s.logger.InfoContext(ctx, "CheckAndDeductCredit RPC called", "userID", req.UserId, "amount", req.AmountToDeduct)

	domainTxType := mapProtoTransactionTypeToDomain(req.TransactionType)
	if domainTxType == "" {
        return nil, status.Errorf(codes.InvalidArgument, "invalid transaction type: %s", req.TransactionType.String())
    }

    var relatedMsgIdPtr *string
    if req.RelatedMessageId != "" {
        relatedMsgIdPtr = &req.RelatedMessageId
    }

	tx, err := s.billingApp.CheckAndDeductCredit(ctx, req.UserId, req.AmountToDeduct, domainTxType, req.Description, relatedMsgIdPtr)
	if err != nil {
		s.logger.ErrorContext(ctx, "CheckAndDeductCredit failed in app layer", "error", err, "userID", req.UserId)
		if errors.Is(err, app.ErrInsufficientCredit) {
			return nil, status.Errorf(codes.FailedPrecondition, err.Error())
		}
        if errors.Is(err, app.ErrUserNotFoundForBilling) { // This error isn't actually returned by current app layer but good to anticipate
            return nil, status.Errorf(codes.NotFound, err.Error())
        }
		return nil, status.Errorf(codes.Internal, "billing operation failed: %v", err)
	}

	protoTx := &billingservice.TransactionProto{
		Id:                 tx.ID,
		UserId:             tx.UserID,
		Type:               mapDomainTransactionTypeToProto(tx.Type),
		Amount:             tx.Amount,
		CurrencyCode:       tx.CurrencyCode,
		Description:        tx.Description,
	}
    if tx.RelatedMessageID != nil { protoTx.RelatedMessageId = *tx.RelatedMessageID }
    if tx.PaymentGatewayTxnID != nil { protoTx.PaymentGatewayTxnId = *tx.PaymentGatewayTxnID }
    if tx.BalanceAfter != nil { protoTx.BalanceAfter = *tx.BalanceAfter }
    if !tx.CreatedAt.IsZero() { protoTx.CreatedAt = timestamppb.New(tx.CreatedAt) }


	return &billingservice.CheckAndDeductCreditResponse{Transaction: protoTx}, nil
}
