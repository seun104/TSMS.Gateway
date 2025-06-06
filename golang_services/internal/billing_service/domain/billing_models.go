package domain

import (
	"database/sql/driver"
	"fmt"
	"time"
	"github.com/google/uuid" // Uncommented
)

// TransactionType defines the nature of a financial transaction.
type TransactionType string

const (
	TransactionTypeCreditPurchase      TransactionType = "credit_purchase"    // From payment gateway, etc.
	TransactionTypeSMSCharge           TransactionType = "sms_charge"         // Cost of sending SMS
	TransactionTypeRefund              TransactionType = "refund"             // Refund for failed services, etc.
	TransactionTypeServiceActivationFee TransactionType = "service_activation_fee"
	TransactionTypeManualAdjustment    TransactionType = "manual_adjustment"    // Admin adjustment
	TransactionTypeCredit              TransactionType = "credit"             // Generic credit (used by payment gateway)
	TransactionTypeDebit               TransactionType = "debit"              // Generic debit (will be used by SMS charge)
	// Add other types as needed
)

// Value implements the driver.Valuer interface for TransactionType.
func (tt TransactionType) Value() (driver.Value, error) {
	return string(tt), nil
}

// Scan implements the sql.Scanner interface for TransactionType.
func (tt *TransactionType) Scan(value interface{}) error {
	strVal, ok := value.(string)
	if !ok {
		bytesVal, ok := value.([]byte)
		if !ok {
			return fmt.Errorf("failed to scan TransactionType: value is not string or []byte, it is %T", value)
		}
		strVal = string(bytesVal)
	}
	parsedTT := TransactionType(strVal)
	// Validate against known enum values
	switch parsedTT {
	case TransactionTypeCreditPurchase, TransactionTypeSMSCharge, TransactionTypeRefund,
		TransactionTypeServiceActivationFee, TransactionTypeManualAdjustment,
		TransactionTypeCredit, TransactionTypeDebit:
		*tt = parsedTT
		return nil
	default:
		return fmt.Errorf("unknown TransactionType value: %s", strVal)
	}
}


// Transaction represents a financial transaction in the system.
type Transaction struct {
	ID                     string          `json:"id"` // UUID
	UserID                 string          `json:"user_id"` // UUID of the user
	Type                   TransactionType `json:"type"`
	Amount                 float64         `json:"amount"` // Represents the magnitude. Type (Credit/Debit) indicates direction.
	CurrencyCode           string          `json:"currency_code"`
	Description            string          `json:"description,omitempty"`
	RelatedMessageID       *string         `json:"related_message_id,omitempty"` // UUID of OutboxMessage if applicable
	PaymentGatewayTxnID    *string         `json:"payment_gateway_txn_id,omitempty"` // If from a payment gateway
	PaymentIntentID        *uuid.UUID      `json:"payment_intent_id,omitempty"`    // Link to our internal payment_intents table
	BalanceBefore          float64         `json:"balance_before"` // User's balance before this transaction
	BalanceAfter           float64         `json:"balance_after"`  // User's balance after this transaction
	CreatedAt              time.Time       `json:"created_at"`
}

// PaymentGateway (if needed within billing domain, or can be a separate config entity)
// type PaymentGateway struct {
//  ID        string    `json:"id"`
//  Name      string    `json:"name"`
//  Config    string    `json:"-"` // JSONB in DB for API keys, URLs
//  IsActive  bool      `json:"is_active"`
//  CreatedAt time.Time `json:"created_at"`
//  UpdatedAt time.Time `json:"updated_at"`
// }
