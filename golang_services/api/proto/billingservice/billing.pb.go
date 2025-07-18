// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.32.0
// 	protoc        v3.21.12
// source: api/proto/billingservice/billing.proto

package billingservice

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type TransactionTypeProto int32

const (
	TransactionTypeProto_TRANSACTION_TYPE_PROTO_UNSPECIFIED            TransactionTypeProto = 0
	TransactionTypeProto_TRANSACTION_TYPE_PROTO_CREDIT_PURCHASE        TransactionTypeProto = 1
	TransactionTypeProto_TRANSACTION_TYPE_PROTO_SMS_CHARGE             TransactionTypeProto = 2
	TransactionTypeProto_TRANSACTION_TYPE_PROTO_REFUND                 TransactionTypeProto = 3
	TransactionTypeProto_TRANSACTION_TYPE_PROTO_SERVICE_ACTIVATION_FEE TransactionTypeProto = 4
	TransactionTypeProto_TRANSACTION_TYPE_PROTO_MANUAL_ADJUSTMENT      TransactionTypeProto = 5
	TransactionTypeProto_TRANSACTION_TYPE_PROTO_CREDIT                 TransactionTypeProto = 6 // Added from previous subtask
	TransactionTypeProto_TRANSACTION_TYPE_PROTO_DEBIT                  TransactionTypeProto = 7 // Added from previous subtask
)

// Enum value maps for TransactionTypeProto.
var (
	TransactionTypeProto_name = map[int32]string{
		0: "TRANSACTION_TYPE_PROTO_UNSPECIFIED",
		1: "TRANSACTION_TYPE_PROTO_CREDIT_PURCHASE",
		2: "TRANSACTION_TYPE_PROTO_SMS_CHARGE",
		3: "TRANSACTION_TYPE_PROTO_REFUND",
		4: "TRANSACTION_TYPE_PROTO_SERVICE_ACTIVATION_FEE",
		5: "TRANSACTION_TYPE_PROTO_MANUAL_ADJUSTMENT",
		6: "TRANSACTION_TYPE_PROTO_CREDIT",
		7: "TRANSACTION_TYPE_PROTO_DEBIT",
	}
	TransactionTypeProto_value = map[string]int32{
		"TRANSACTION_TYPE_PROTO_UNSPECIFIED":            0,
		"TRANSACTION_TYPE_PROTO_CREDIT_PURCHASE":        1,
		"TRANSACTION_TYPE_PROTO_SMS_CHARGE":             2,
		"TRANSACTION_TYPE_PROTO_REFUND":                 3,
		"TRANSACTION_TYPE_PROTO_SERVICE_ACTIVATION_FEE": 4,
		"TRANSACTION_TYPE_PROTO_MANUAL_ADJUSTMENT":      5,
		"TRANSACTION_TYPE_PROTO_CREDIT":                 6,
		"TRANSACTION_TYPE_PROTO_DEBIT":                  7,
	}
)

func (x TransactionTypeProto) Enum() *TransactionTypeProto {
	p := new(TransactionTypeProto)
	*p = x
	return p
}

func (x TransactionTypeProto) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (TransactionTypeProto) Descriptor() protoreflect.EnumDescriptor {
	return file_api_proto_billingservice_billing_proto_enumTypes[0].Descriptor()
}

func (TransactionTypeProto) Type() protoreflect.EnumType {
	return &file_api_proto_billingservice_billing_proto_enumTypes[0]
}

func (x TransactionTypeProto) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use TransactionTypeProto.Descriptor instead.
func (TransactionTypeProto) EnumDescriptor() ([]byte, []int) {
	return file_api_proto_billingservice_billing_proto_rawDescGZIP(), []int{0}
}

type TransactionProto struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Id                  string                 `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	UserId              string                 `protobuf:"bytes,2,opt,name=user_id,json=userId,proto3" json:"user_id,omitempty"`
	Type                TransactionTypeProto   `protobuf:"varint,3,opt,name=type,proto3,enum=billingservice.TransactionTypeProto" json:"type,omitempty"`
	Amount              float64                `protobuf:"fixed64,4,opt,name=amount,proto3" json:"amount,omitempty"` // Consider string for precision or custom decimal type
	CurrencyCode        string                 `protobuf:"bytes,5,opt,name=currency_code,json=currencyCode,proto3" json:"currency_code,omitempty"`
	Description         string                 `protobuf:"bytes,6,opt,name=description,proto3" json:"description,omitempty"`
	RelatedMessageId    string                 `protobuf:"bytes,7,opt,name=related_message_id,json=relatedMessageId,proto3" json:"related_message_id,omitempty"`            // Optional
	PaymentGatewayTxnId string                 `protobuf:"bytes,8,opt,name=payment_gateway_txn_id,json=paymentGatewayTxnId,proto3" json:"payment_gateway_txn_id,omitempty"` // Optional
	CreatedAt           *timestamppb.Timestamp `protobuf:"bytes,9,opt,name=created_at,json=createdAt,proto3" json:"created_at,omitempty"`
	BalanceAfter        float64                `protobuf:"fixed64,10,opt,name=balance_after,json=balanceAfter,proto3" json:"balance_after,omitempty"` // Optional
}

func (x *TransactionProto) Reset() {
	*x = TransactionProto{}
	if protoimpl.UnsafeEnabled {
		mi := &file_api_proto_billingservice_billing_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *TransactionProto) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TransactionProto) ProtoMessage() {}

func (x *TransactionProto) ProtoReflect() protoreflect.Message {
	mi := &file_api_proto_billingservice_billing_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TransactionProto.ProtoReflect.Descriptor instead.
func (*TransactionProto) Descriptor() ([]byte, []int) {
	return file_api_proto_billingservice_billing_proto_rawDescGZIP(), []int{0}
}

func (x *TransactionProto) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

func (x *TransactionProto) GetUserId() string {
	if x != nil {
		return x.UserId
	}
	return ""
}

func (x *TransactionProto) GetType() TransactionTypeProto {
	if x != nil {
		return x.Type
	}
	return TransactionTypeProto_TRANSACTION_TYPE_PROTO_UNSPECIFIED
}

func (x *TransactionProto) GetAmount() float64 {
	if x != nil {
		return x.Amount
	}
	return 0
}

func (x *TransactionProto) GetCurrencyCode() string {
	if x != nil {
		return x.CurrencyCode
	}
	return ""
}

func (x *TransactionProto) GetDescription() string {
	if x != nil {
		return x.Description
	}
	return ""
}

func (x *TransactionProto) GetRelatedMessageId() string {
	if x != nil {
		return x.RelatedMessageId
	}
	return ""
}

func (x *TransactionProto) GetPaymentGatewayTxnId() string {
	if x != nil {
		return x.PaymentGatewayTxnId
	}
	return ""
}

func (x *TransactionProto) GetCreatedAt() *timestamppb.Timestamp {
	if x != nil {
		return x.CreatedAt
	}
	return nil
}

func (x *TransactionProto) GetBalanceAfter() float64 {
	if x != nil {
		return x.BalanceAfter
	}
	return 0
}

type CheckAndDeductCreditRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	UserId string `protobuf:"bytes,1,opt,name=user_id,json=userId,proto3" json:"user_id,omitempty"`
	// double amount_to_deduct = 2; // Old field
	SegmentsToCharge int32  `protobuf:"varint,2,opt,name=segments_to_charge,json=segmentsToCharge,proto3" json:"segments_to_charge,omitempty"` // New field: number of message segments or units to charge for
	Description      string `protobuf:"bytes,3,opt,name=description,proto3" json:"description,omitempty"`                                      // Kept, can be augmented by server
	ReferenceId      string `protobuf:"bytes,4,opt,name=reference_id,json=referenceId,proto3" json:"reference_id,omitempty"`                   // Kept, e.g., outbox_message_id
}

func (x *CheckAndDeductCreditRequest) Reset() {
	*x = CheckAndDeductCreditRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_api_proto_billingservice_billing_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *CheckAndDeductCreditRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CheckAndDeductCreditRequest) ProtoMessage() {}

func (x *CheckAndDeductCreditRequest) ProtoReflect() protoreflect.Message {
	mi := &file_api_proto_billingservice_billing_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CheckAndDeductCreditRequest.ProtoReflect.Descriptor instead.
func (*CheckAndDeductCreditRequest) Descriptor() ([]byte, []int) {
	return file_api_proto_billingservice_billing_proto_rawDescGZIP(), []int{1}
}

func (x *CheckAndDeductCreditRequest) GetUserId() string {
	if x != nil {
		return x.UserId
	}
	return ""
}

func (x *CheckAndDeductCreditRequest) GetSegmentsToCharge() int32 {
	if x != nil {
		return x.SegmentsToCharge
	}
	return 0
}

func (x *CheckAndDeductCreditRequest) GetDescription() string {
	if x != nil {
		return x.Description
	}
	return ""
}

func (x *CheckAndDeductCreditRequest) GetReferenceId() string {
	if x != nil {
		return x.ReferenceId
	}
	return ""
}

type CheckAndDeductCreditResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Success        bool              `protobuf:"varint,1,opt,name=success,proto3" json:"success,omitempty"`
	ErrorMessage   string            `protobuf:"bytes,2,opt,name=error_message,json=errorMessage,proto3" json:"error_message,omitempty"`        // If success is false
	TransactionId  string            `protobuf:"bytes,3,opt,name=transaction_id,json=transactionId,proto3" json:"transaction_id,omitempty"`     // ID of the recorded transaction (if successful)
	AmountDeducted int64             `protobuf:"varint,4,opt,name=amount_deducted,json=amountDeducted,proto3" json:"amount_deducted,omitempty"` // Actual amount deducted, in smallest currency unit
	Currency       string            `protobuf:"bytes,5,opt,name=currency,proto3" json:"currency,omitempty"`                                    // Currency of the amount_deducted
	Transaction    *TransactionProto `protobuf:"bytes,6,opt,name=transaction,proto3" json:"transaction,omitempty"`                              // Optional: return the full created transaction proto
}

func (x *CheckAndDeductCreditResponse) Reset() {
	*x = CheckAndDeductCreditResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_api_proto_billingservice_billing_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *CheckAndDeductCreditResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CheckAndDeductCreditResponse) ProtoMessage() {}

func (x *CheckAndDeductCreditResponse) ProtoReflect() protoreflect.Message {
	mi := &file_api_proto_billingservice_billing_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CheckAndDeductCreditResponse.ProtoReflect.Descriptor instead.
func (*CheckAndDeductCreditResponse) Descriptor() ([]byte, []int) {
	return file_api_proto_billingservice_billing_proto_rawDescGZIP(), []int{2}
}

func (x *CheckAndDeductCreditResponse) GetSuccess() bool {
	if x != nil {
		return x.Success
	}
	return false
}

func (x *CheckAndDeductCreditResponse) GetErrorMessage() string {
	if x != nil {
		return x.ErrorMessage
	}
	return ""
}

func (x *CheckAndDeductCreditResponse) GetTransactionId() string {
	if x != nil {
		return x.TransactionId
	}
	return ""
}

func (x *CheckAndDeductCreditResponse) GetAmountDeducted() int64 {
	if x != nil {
		return x.AmountDeducted
	}
	return 0
}

func (x *CheckAndDeductCreditResponse) GetCurrency() string {
	if x != nil {
		return x.Currency
	}
	return ""
}

func (x *CheckAndDeductCreditResponse) GetTransaction() *TransactionProto {
	if x != nil {
		return x.Transaction
	}
	return nil
}

var File_api_proto_billingservice_billing_proto protoreflect.FileDescriptor

var file_api_proto_billingservice_billing_proto_rawDesc = []byte{
	0x0a, 0x26, 0x61, 0x70, 0x69, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x62, 0x69, 0x6c, 0x6c,
	0x69, 0x6e, 0x67, 0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x2f, 0x62, 0x69, 0x6c, 0x6c, 0x69,
	0x6e, 0x67, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x0e, 0x62, 0x69, 0x6c, 0x6c, 0x69, 0x6e,
	0x67, 0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x1a, 0x1f, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x74, 0x69, 0x6d, 0x65, 0x73, 0x74,
	0x61, 0x6d, 0x70, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x97, 0x03, 0x0a, 0x10, 0x54, 0x72,
	0x61, 0x6e, 0x73, 0x61, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x50, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x0e,
	0x0a, 0x02, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x02, 0x69, 0x64, 0x12, 0x17,
	0x0a, 0x07, 0x75, 0x73, 0x65, 0x72, 0x5f, 0x69, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x06, 0x75, 0x73, 0x65, 0x72, 0x49, 0x64, 0x12, 0x38, 0x0a, 0x04, 0x74, 0x79, 0x70, 0x65, 0x18,
	0x03, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x24, 0x2e, 0x62, 0x69, 0x6c, 0x6c, 0x69, 0x6e, 0x67, 0x73,
	0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x2e, 0x54, 0x72, 0x61, 0x6e, 0x73, 0x61, 0x63, 0x74, 0x69,
	0x6f, 0x6e, 0x54, 0x79, 0x70, 0x65, 0x50, 0x72, 0x6f, 0x74, 0x6f, 0x52, 0x04, 0x74, 0x79, 0x70,
	0x65, 0x12, 0x16, 0x0a, 0x06, 0x61, 0x6d, 0x6f, 0x75, 0x6e, 0x74, 0x18, 0x04, 0x20, 0x01, 0x28,
	0x01, 0x52, 0x06, 0x61, 0x6d, 0x6f, 0x75, 0x6e, 0x74, 0x12, 0x23, 0x0a, 0x0d, 0x63, 0x75, 0x72,
	0x72, 0x65, 0x6e, 0x63, 0x79, 0x5f, 0x63, 0x6f, 0x64, 0x65, 0x18, 0x05, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x0c, 0x63, 0x75, 0x72, 0x72, 0x65, 0x6e, 0x63, 0x79, 0x43, 0x6f, 0x64, 0x65, 0x12, 0x20,
	0x0a, 0x0b, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x06, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x0b, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e,
	0x12, 0x2c, 0x0a, 0x12, 0x72, 0x65, 0x6c, 0x61, 0x74, 0x65, 0x64, 0x5f, 0x6d, 0x65, 0x73, 0x73,
	0x61, 0x67, 0x65, 0x5f, 0x69, 0x64, 0x18, 0x07, 0x20, 0x01, 0x28, 0x09, 0x52, 0x10, 0x72, 0x65,
	0x6c, 0x61, 0x74, 0x65, 0x64, 0x4d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x49, 0x64, 0x12, 0x33,
	0x0a, 0x16, 0x70, 0x61, 0x79, 0x6d, 0x65, 0x6e, 0x74, 0x5f, 0x67, 0x61, 0x74, 0x65, 0x77, 0x61,
	0x79, 0x5f, 0x74, 0x78, 0x6e, 0x5f, 0x69, 0x64, 0x18, 0x08, 0x20, 0x01, 0x28, 0x09, 0x52, 0x13,
	0x70, 0x61, 0x79, 0x6d, 0x65, 0x6e, 0x74, 0x47, 0x61, 0x74, 0x65, 0x77, 0x61, 0x79, 0x54, 0x78,
	0x6e, 0x49, 0x64, 0x12, 0x39, 0x0a, 0x0a, 0x63, 0x72, 0x65, 0x61, 0x74, 0x65, 0x64, 0x5f, 0x61,
	0x74, 0x18, 0x09, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x54, 0x69, 0x6d, 0x65, 0x73, 0x74,
	0x61, 0x6d, 0x70, 0x52, 0x09, 0x63, 0x72, 0x65, 0x61, 0x74, 0x65, 0x64, 0x41, 0x74, 0x12, 0x23,
	0x0a, 0x0d, 0x62, 0x61, 0x6c, 0x61, 0x6e, 0x63, 0x65, 0x5f, 0x61, 0x66, 0x74, 0x65, 0x72, 0x18,
	0x0a, 0x20, 0x01, 0x28, 0x01, 0x52, 0x0c, 0x62, 0x61, 0x6c, 0x61, 0x6e, 0x63, 0x65, 0x41, 0x66,
	0x74, 0x65, 0x72, 0x22, 0xa9, 0x01, 0x0a, 0x1b, 0x43, 0x68, 0x65, 0x63, 0x6b, 0x41, 0x6e, 0x64,
	0x44, 0x65, 0x64, 0x75, 0x63, 0x74, 0x43, 0x72, 0x65, 0x64, 0x69, 0x74, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x12, 0x17, 0x0a, 0x07, 0x75, 0x73, 0x65, 0x72, 0x5f, 0x69, 0x64, 0x18, 0x01,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x75, 0x73, 0x65, 0x72, 0x49, 0x64, 0x12, 0x2c, 0x0a, 0x12,
	0x73, 0x65, 0x67, 0x6d, 0x65, 0x6e, 0x74, 0x73, 0x5f, 0x74, 0x6f, 0x5f, 0x63, 0x68, 0x61, 0x72,
	0x67, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x05, 0x52, 0x10, 0x73, 0x65, 0x67, 0x6d, 0x65, 0x6e,
	0x74, 0x73, 0x54, 0x6f, 0x43, 0x68, 0x61, 0x72, 0x67, 0x65, 0x12, 0x20, 0x0a, 0x0b, 0x64, 0x65,
	0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x0b, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x21, 0x0a, 0x0c,
	0x72, 0x65, 0x66, 0x65, 0x72, 0x65, 0x6e, 0x63, 0x65, 0x5f, 0x69, 0x64, 0x18, 0x04, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x0b, 0x72, 0x65, 0x66, 0x65, 0x72, 0x65, 0x6e, 0x63, 0x65, 0x49, 0x64, 0x22,
	0x8d, 0x02, 0x0a, 0x1c, 0x43, 0x68, 0x65, 0x63, 0x6b, 0x41, 0x6e, 0x64, 0x44, 0x65, 0x64, 0x75,
	0x63, 0x74, 0x43, 0x72, 0x65, 0x64, 0x69, 0x74, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x12, 0x18, 0x0a, 0x07, 0x73, 0x75, 0x63, 0x63, 0x65, 0x73, 0x73, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x08, 0x52, 0x07, 0x73, 0x75, 0x63, 0x63, 0x65, 0x73, 0x73, 0x12, 0x23, 0x0a, 0x0d, 0x65, 0x72,
	0x72, 0x6f, 0x72, 0x5f, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x0c, 0x65, 0x72, 0x72, 0x6f, 0x72, 0x4d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x12,
	0x25, 0x0a, 0x0e, 0x74, 0x72, 0x61, 0x6e, 0x73, 0x61, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x5f, 0x69,
	0x64, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0d, 0x74, 0x72, 0x61, 0x6e, 0x73, 0x61, 0x63,
	0x74, 0x69, 0x6f, 0x6e, 0x49, 0x64, 0x12, 0x27, 0x0a, 0x0f, 0x61, 0x6d, 0x6f, 0x75, 0x6e, 0x74,
	0x5f, 0x64, 0x65, 0x64, 0x75, 0x63, 0x74, 0x65, 0x64, 0x18, 0x04, 0x20, 0x01, 0x28, 0x03, 0x52,
	0x0e, 0x61, 0x6d, 0x6f, 0x75, 0x6e, 0x74, 0x44, 0x65, 0x64, 0x75, 0x63, 0x74, 0x65, 0x64, 0x12,
	0x1a, 0x0a, 0x08, 0x63, 0x75, 0x72, 0x72, 0x65, 0x6e, 0x63, 0x79, 0x18, 0x05, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x08, 0x63, 0x75, 0x72, 0x72, 0x65, 0x6e, 0x63, 0x79, 0x12, 0x42, 0x0a, 0x0b, 0x74,
	0x72, 0x61, 0x6e, 0x73, 0x61, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x06, 0x20, 0x01, 0x28, 0x0b,
	0x32, 0x20, 0x2e, 0x62, 0x69, 0x6c, 0x6c, 0x69, 0x6e, 0x67, 0x73, 0x65, 0x72, 0x76, 0x69, 0x63,
	0x65, 0x2e, 0x54, 0x72, 0x61, 0x6e, 0x73, 0x61, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x50, 0x72, 0x6f,
	0x74, 0x6f, 0x52, 0x0b, 0x74, 0x72, 0x61, 0x6e, 0x73, 0x61, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x2a,
	0xda, 0x02, 0x0a, 0x14, 0x54, 0x72, 0x61, 0x6e, 0x73, 0x61, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x54,
	0x79, 0x70, 0x65, 0x50, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x26, 0x0a, 0x22, 0x54, 0x52, 0x41, 0x4e,
	0x53, 0x41, 0x43, 0x54, 0x49, 0x4f, 0x4e, 0x5f, 0x54, 0x59, 0x50, 0x45, 0x5f, 0x50, 0x52, 0x4f,
	0x54, 0x4f, 0x5f, 0x55, 0x4e, 0x53, 0x50, 0x45, 0x43, 0x49, 0x46, 0x49, 0x45, 0x44, 0x10, 0x00,
	0x12, 0x2a, 0x0a, 0x26, 0x54, 0x52, 0x41, 0x4e, 0x53, 0x41, 0x43, 0x54, 0x49, 0x4f, 0x4e, 0x5f,
	0x54, 0x59, 0x50, 0x45, 0x5f, 0x50, 0x52, 0x4f, 0x54, 0x4f, 0x5f, 0x43, 0x52, 0x45, 0x44, 0x49,
	0x54, 0x5f, 0x50, 0x55, 0x52, 0x43, 0x48, 0x41, 0x53, 0x45, 0x10, 0x01, 0x12, 0x25, 0x0a, 0x21,
	0x54, 0x52, 0x41, 0x4e, 0x53, 0x41, 0x43, 0x54, 0x49, 0x4f, 0x4e, 0x5f, 0x54, 0x59, 0x50, 0x45,
	0x5f, 0x50, 0x52, 0x4f, 0x54, 0x4f, 0x5f, 0x53, 0x4d, 0x53, 0x5f, 0x43, 0x48, 0x41, 0x52, 0x47,
	0x45, 0x10, 0x02, 0x12, 0x21, 0x0a, 0x1d, 0x54, 0x52, 0x41, 0x4e, 0x53, 0x41, 0x43, 0x54, 0x49,
	0x4f, 0x4e, 0x5f, 0x54, 0x59, 0x50, 0x45, 0x5f, 0x50, 0x52, 0x4f, 0x54, 0x4f, 0x5f, 0x52, 0x45,
	0x46, 0x55, 0x4e, 0x44, 0x10, 0x03, 0x12, 0x31, 0x0a, 0x2d, 0x54, 0x52, 0x41, 0x4e, 0x53, 0x41,
	0x43, 0x54, 0x49, 0x4f, 0x4e, 0x5f, 0x54, 0x59, 0x50, 0x45, 0x5f, 0x50, 0x52, 0x4f, 0x54, 0x4f,
	0x5f, 0x53, 0x45, 0x52, 0x56, 0x49, 0x43, 0x45, 0x5f, 0x41, 0x43, 0x54, 0x49, 0x56, 0x41, 0x54,
	0x49, 0x4f, 0x4e, 0x5f, 0x46, 0x45, 0x45, 0x10, 0x04, 0x12, 0x2c, 0x0a, 0x28, 0x54, 0x52, 0x41,
	0x4e, 0x53, 0x41, 0x43, 0x54, 0x49, 0x4f, 0x4e, 0x5f, 0x54, 0x59, 0x50, 0x45, 0x5f, 0x50, 0x52,
	0x4f, 0x54, 0x4f, 0x5f, 0x4d, 0x41, 0x4e, 0x55, 0x41, 0x4c, 0x5f, 0x41, 0x44, 0x4a, 0x55, 0x53,
	0x54, 0x4d, 0x45, 0x4e, 0x54, 0x10, 0x05, 0x12, 0x21, 0x0a, 0x1d, 0x54, 0x52, 0x41, 0x4e, 0x53,
	0x41, 0x43, 0x54, 0x49, 0x4f, 0x4e, 0x5f, 0x54, 0x59, 0x50, 0x45, 0x5f, 0x50, 0x52, 0x4f, 0x54,
	0x4f, 0x5f, 0x43, 0x52, 0x45, 0x44, 0x49, 0x54, 0x10, 0x06, 0x12, 0x20, 0x0a, 0x1c, 0x54, 0x52,
	0x41, 0x4e, 0x53, 0x41, 0x43, 0x54, 0x49, 0x4f, 0x4e, 0x5f, 0x54, 0x59, 0x50, 0x45, 0x5f, 0x50,
	0x52, 0x4f, 0x54, 0x4f, 0x5f, 0x44, 0x45, 0x42, 0x49, 0x54, 0x10, 0x07, 0x32, 0x8b, 0x01, 0x0a,
	0x16, 0x42, 0x69, 0x6c, 0x6c, 0x69, 0x6e, 0x67, 0x53, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x49,
	0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x12, 0x71, 0x0a, 0x14, 0x43, 0x68, 0x65, 0x63, 0x6b,
	0x41, 0x6e, 0x64, 0x44, 0x65, 0x64, 0x75, 0x63, 0x74, 0x43, 0x72, 0x65, 0x64, 0x69, 0x74, 0x12,
	0x2b, 0x2e, 0x62, 0x69, 0x6c, 0x6c, 0x69, 0x6e, 0x67, 0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65,
	0x2e, 0x43, 0x68, 0x65, 0x63, 0x6b, 0x41, 0x6e, 0x64, 0x44, 0x65, 0x64, 0x75, 0x63, 0x74, 0x43,
	0x72, 0x65, 0x64, 0x69, 0x74, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x2c, 0x2e, 0x62,
	0x69, 0x6c, 0x6c, 0x69, 0x6e, 0x67, 0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x2e, 0x43, 0x68,
	0x65, 0x63, 0x6b, 0x41, 0x6e, 0x64, 0x44, 0x65, 0x64, 0x75, 0x63, 0x74, 0x43, 0x72, 0x65, 0x64,
	0x69, 0x74, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x42, 0x44, 0x5a, 0x42, 0x67, 0x69,
	0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x41, 0x72, 0x61, 0x64, 0x49, 0x54, 0x2f,
	0x61, 0x72, 0x61, 0x64, 0x73, 0x6d, 0x73, 0x2f, 0x67, 0x6f, 0x6c, 0x61, 0x6e, 0x67, 0x5f, 0x73,
	0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x73, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x2f, 0x62, 0x69, 0x6c, 0x6c, 0x69, 0x6e, 0x67, 0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65,
	0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_api_proto_billingservice_billing_proto_rawDescOnce sync.Once
	file_api_proto_billingservice_billing_proto_rawDescData = file_api_proto_billingservice_billing_proto_rawDesc
)

func file_api_proto_billingservice_billing_proto_rawDescGZIP() []byte {
	file_api_proto_billingservice_billing_proto_rawDescOnce.Do(func() {
		file_api_proto_billingservice_billing_proto_rawDescData = protoimpl.X.CompressGZIP(file_api_proto_billingservice_billing_proto_rawDescData)
	})
	return file_api_proto_billingservice_billing_proto_rawDescData
}

var file_api_proto_billingservice_billing_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_api_proto_billingservice_billing_proto_msgTypes = make([]protoimpl.MessageInfo, 3)
var file_api_proto_billingservice_billing_proto_goTypes = []interface{}{
	(TransactionTypeProto)(0),            // 0: billingservice.TransactionTypeProto
	(*TransactionProto)(nil),             // 1: billingservice.TransactionProto
	(*CheckAndDeductCreditRequest)(nil),  // 2: billingservice.CheckAndDeductCreditRequest
	(*CheckAndDeductCreditResponse)(nil), // 3: billingservice.CheckAndDeductCreditResponse
	(*timestamppb.Timestamp)(nil),        // 4: google.protobuf.Timestamp
}
var file_api_proto_billingservice_billing_proto_depIdxs = []int32{
	0, // 0: billingservice.TransactionProto.type:type_name -> billingservice.TransactionTypeProto
	4, // 1: billingservice.TransactionProto.created_at:type_name -> google.protobuf.Timestamp
	1, // 2: billingservice.CheckAndDeductCreditResponse.transaction:type_name -> billingservice.TransactionProto
	2, // 3: billingservice.BillingServiceInternal.CheckAndDeductCredit:input_type -> billingservice.CheckAndDeductCreditRequest
	3, // 4: billingservice.BillingServiceInternal.CheckAndDeductCredit:output_type -> billingservice.CheckAndDeductCreditResponse
	4, // [4:5] is the sub-list for method output_type
	3, // [3:4] is the sub-list for method input_type
	3, // [3:3] is the sub-list for extension type_name
	3, // [3:3] is the sub-list for extension extendee
	0, // [0:3] is the sub-list for field type_name
}

func init() { file_api_proto_billingservice_billing_proto_init() }
func file_api_proto_billingservice_billing_proto_init() {
	if File_api_proto_billingservice_billing_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_api_proto_billingservice_billing_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*TransactionProto); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_api_proto_billingservice_billing_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*CheckAndDeductCreditRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_api_proto_billingservice_billing_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*CheckAndDeductCreditResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_api_proto_billingservice_billing_proto_rawDesc,
			NumEnums:      1,
			NumMessages:   3,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_api_proto_billingservice_billing_proto_goTypes,
		DependencyIndexes: file_api_proto_billingservice_billing_proto_depIdxs,
		EnumInfos:         file_api_proto_billingservice_billing_proto_enumTypes,
		MessageInfos:      file_api_proto_billingservice_billing_proto_msgTypes,
	}.Build()
	File_api_proto_billingservice_billing_proto = out.File
	file_api_proto_billingservice_billing_proto_rawDesc = nil
	file_api_proto_billingservice_billing_proto_goTypes = nil
	file_api_proto_billingservice_billing_proto_depIdxs = nil
}
