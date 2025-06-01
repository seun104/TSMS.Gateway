package domain

import (
	"time"
	// "github.com/google/uuid"
)

// TransactionType defines the nature of a financial transaction.
type TransactionType string

const (
	TransactionTypeCreditPurchase      TransactionType = "credit_purchase"
	TransactionTypeSMSCharge           TransactionType = "sms_charge"
	TransactionTypeRefund              TransactionType = "refund"
	TransactionTypeServiceActivation TransactionType = "service_activation_fee"
	TransactionTypeManualAdjustment    TransactionType = "manual_adjustment"
)

// Transaction represents a financial event in the system.
type Transaction struct {
	ID                 string          `json:"id"` // UUID
	UserID             string          `json:"user_id"` // UUID of the user
	Type               TransactionType `json:"type"`
	Amount             float64         `json:"amount"` // Positive for credit, negative for debit. Consider decimal type for precision.
	CurrencyCode       string          `json:"currency_code"`
	Description        string          `json:"description,omitempty"`
	RelatedMessageID   *string         `json:"related_message_id,omitempty"` // UUID of an outbox message if applicable
	PaymentGatewayTxnID *string        `json:"payment_gateway_txn_id,omitempty"` // If from a payment gateway
	CreatedAt          time.Time       `json:"created_at"`
    BalanceAfter       *float64        `json:"balance_after,omitempty"` // Optional: User's balance after this transaction
}
