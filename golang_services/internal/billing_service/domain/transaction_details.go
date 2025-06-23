package domain

// TransactionDetails provides context for a transaction.
type TransactionDetails struct {
	Description string // More detailed description if needed
	ReferenceID string // ID of the related entity (e.g., outbox_message_id, payment_intent_id)
}
