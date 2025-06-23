package grpc

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/your-repo/project/internal/phonebook_service/app"
	"github.com/your-repo/project/internal/phonebook_service/domain"
	pb "github.com/your-repo/project/golang_services/api/proto/phonebookservice" // Generated proto
)

// GRPCServer implements the phonebookservice.PhonebookServiceServer interface.
type GRPCServer struct {
	pb.UnimplementedPhonebookServiceServer // Recommended for forward compatibility
	app                                    *app.Application
	logger                                 *slog.Logger
}

// NewGRPCServer creates a new GRPCServer instance.
func NewGRPCServer(application *app.Application, logger *slog.Logger) *GRPCServer {
	return &GRPCServer{
		app:    application,
		logger: logger,
	}
}

// Placeholder for userID extraction from context. In a real app, this would involve JWT/auth middleware.
func getUserIDFromCtx(ctx context.Context) (uuid.UUID, error) {
	// For now, returning a fixed test UserID. Replace with actual auth logic.
	// testUserIDStr := "00000000-0000-0000-0000-000000000001"
	// userID, err := uuid.Parse(testUserIDStr)
	// if err != nil {
	// 	return uuid.Nil, status.Errorf(codes.Internal, "failed to parse test user ID: %v", err)
	// }
	// return userID, nil

	// To allow tests without full auth, let's check for a context value or return Nil
	userIDVal := ctx.Value("user_id") // This is a placeholder for actual auth context key
	if userIDVal != nil {
		if userIDStr, ok := userIDVal.(string); ok {
			parsedUUID, err := uuid.Parse(userIDStr)
			if err == nil {
				return parsedUUID, nil
			}
		}
		if userID, ok := userIDVal.(uuid.UUID); ok {
			return userID, nil
		}
	}
	// If no user_id in context, or it's invalid, operations that require user_id might fail or act as anonymous.
	// For phonebook, most ops are user-scoped, so an error or Nil UUID is appropriate.
	// Returning Nil UUID for now, application layer should handle if it's required.
	// return uuid.Nil, status.Errorf(codes.Unauthenticated, "user ID not found in context")
	// For initial testing, let's use a default if not found to allow basic CRUD without full auth setup.
	 defaultTestUserID := "123e4567-e89b-12d3-a456-426614174000" // A valid UUID string
	 userID, _ := uuid.Parse(defaultTestUserID)
	 return userID, nil

}


// --- Phonebook RPC Implementations ---

func (s *GRPCServer) CreatePhonebook(ctx context.Context, req *pb.CreatePhonebookRequest) (*pb.CreatePhonebookResponse, error) {
	userID, err := getUserIDFromCtx(ctx)
	if err != nil {
		return nil, err // Error already gRPC status formatted
	}
	if userID == uuid.Nil {
		return nil, status.Errorf(codes.Unauthenticated, "user authentication required")
	}

	domainPb, err := s.app.CreatePhonebook(ctx, userID, req.GetName(), req.GetDescription())
	if err != nil {
		s.logger.ErrorContext(ctx, "CreatePhonebook app error", "error", err)
		// TODO: Map domain errors to gRPC status codes more granularly
		return nil, status.Errorf(codes.Internal, "failed to create phonebook: %v", err)
	}
	return &pb.CreatePhonebookResponse{Phonebook: toProtoPhonebook(domainPb)}, nil
}

func (s *GRPCServer) GetPhonebook(ctx context.Context, req *pb.GetPhonebookRequest) (*pb.GetPhonebookResponse, error) {
	userID, err := getUserIDFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	if userID == uuid.Nil {
		return nil, status.Errorf(codes.Unauthenticated, "user authentication required")
	}
	phonebookID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid phonebook ID format: %v", err)
	}

	domainPb, err := s.app.GetPhonebook(ctx, phonebookID, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "phonebook not found or access denied")
		}
		s.logger.ErrorContext(ctx, "GetPhonebook app error", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to get phonebook: %v", err)
	}
	return &pb.GetPhonebookResponse{Phonebook: toProtoPhonebook(domainPb)}, nil
}

func (s *GRPCServer) ListPhonebooks(ctx context.Context, req *pb.ListPhonebooksRequest) (*pb.ListPhonebooksResponse, error) {
	userID, err := getUserIDFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	if userID == uuid.Nil {
		return nil, status.Errorf(codes.Unauthenticated, "user authentication required")
	}

	domainPbs, totalCount, err := s.app.ListPhonebooks(ctx, userID, int(req.GetOffset()), int(req.GetLimit()))
	if err != nil {
		s.logger.ErrorContext(ctx, "ListPhonebooks app error", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to list phonebooks: %v", err)
	}

	protoPbs := make([]*pb.Phonebook, len(domainPbs))
	for i, dpb := range domainPbs {
		protoPbs[i] = toProtoPhonebook(dpb)
	}
	return &pb.ListPhonebooksResponse{Phonebooks: protoPbs, TotalCount: int32(totalCount)}, nil
}

func (s *GRPCServer) UpdatePhonebook(ctx context.Context, req *pb.UpdatePhonebookRequest) (*pb.UpdatePhonebookResponse, error) {
	userID, err := getUserIDFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	if userID == uuid.Nil {
		return nil, status.Errorf(codes.Unauthenticated, "user authentication required")
	}
	phonebookID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid phonebook ID format: %v", err)
	}

	domainPb, err := s.app.UpdatePhonebook(ctx, phonebookID, userID, req.GetName(), req.GetDescription())
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "phonebook not found or access denied for update")
		}
		s.logger.ErrorContext(ctx, "UpdatePhonebook app error", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to update phonebook: %v", err)
	}
	return &pb.UpdatePhonebookResponse{Phonebook: toProtoPhonebook(domainPb)}, nil
}

func (s *GRPCServer) DeletePhonebook(ctx context.Context, req *pb.DeletePhonebookRequest) (*emptypb.Empty, error) {
	userID, err := getUserIDFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	if userID == uuid.Nil {
		return nil, status.Errorf(codes.Unauthenticated, "user authentication required")
	}
	phonebookID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid phonebook ID format: %v", err)
	}

	err = s.app.DeletePhonebook(ctx, phonebookID, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "phonebook not found or access denied for delete")
		}
		s.logger.ErrorContext(ctx, "DeletePhonebook app error", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to delete phonebook: %v", err)
	}
	return &emptypb.Empty{}, nil
}

// --- Contact RPC Implementations ---

func (s *GRPCServer) CreateContact(ctx context.Context, req *pb.CreateContactRequest) (*pb.CreateContactResponse, error) {
	userID, err := getUserIDFromCtx(ctx) // User ID for phonebook ownership check
	if err != nil { return nil, err }
	if userID == uuid.Nil { return nil, status.Errorf(codes.Unauthenticated, "user authentication required") }

	phonebookID, err := uuid.Parse(req.GetPhonebookId())
	if err != nil { return nil, status.Errorf(codes.InvalidArgument, "invalid phonebook_id format: %v", err) }

	// Verify user owns the phonebook before adding a contact
	_, err = s.app.GetPhonebook(ctx, phonebookID, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "phonebook not found or access denied")
		}
		s.logger.ErrorContext(ctx, "Error verifying phonebook ownership for CreateContact", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to verify phonebook ownership: %v", err)
	}

	domainContact, err := s.app.CreateContact(ctx, phonebookID, req.GetNumber(), req.GetFirstName(), req.GetLastName(), req.GetEmail(), req.GetCustomFields(), req.GetSubscribed())
	if err != nil {
		if errors.Is(err, domain.ErrDuplicateEntry) {
			return nil, status.Errorf(codes.AlreadyExists, "contact with this number already exists in the phonebook")
		}
		s.logger.ErrorContext(ctx, "CreateContact app error", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to create contact: %v", err)
	}
	return &pb.CreateContactResponse{Contact: toProtoContact(domainContact)}, nil
}

func (s *GRPCServer) GetContact(ctx context.Context, req *pb.GetContactRequest) (*pb.GetContactResponse, error) {
	userID, err := getUserIDFromCtx(ctx)
	if err != nil { return nil, err }
    if userID == uuid.Nil { return nil, status.Errorf(codes.Unauthenticated, "user authentication required") }

	contactID, err := uuid.Parse(req.GetId())
	if err != nil { return nil, status.Errorf(codes.InvalidArgument, "invalid contact ID format: %v", err) }
	phonebookID, err := uuid.Parse(req.GetPhonebookId())
	if err != nil { return nil, status.Errorf(codes.InvalidArgument, "invalid phonebook_id format: %v", err) }

	// Verify user owns the phonebook
	_, err = s.app.GetPhonebook(ctx, phonebookID, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "phonebook not found or access denied, cannot get contact")
		}
		return nil, status.Errorf(codes.Internal, "failed to verify phonebook ownership: %v", err)
	}

	domainContact, err := s.app.GetContact(ctx, contactID, phonebookID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "contact not found")
		}
		s.logger.ErrorContext(ctx, "GetContact app error", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to get contact: %v", err)
	}
	return &pb.GetContactResponse{Contact: toProtoContact(domainContact)}, nil
}

func (s *GRPCServer) ListContacts(ctx context.Context, req *pb.ListContactsRequest) (*pb.ListContactsResponse, error) {
	userID, err := getUserIDFromCtx(ctx)
	if err != nil { return nil, err }
    if userID == uuid.Nil { return nil, status.Errorf(codes.Unauthenticated, "user authentication required") }

	phonebookID, err := uuid.Parse(req.GetPhonebookId())
	if err != nil { return nil, status.Errorf(codes.InvalidArgument, "invalid phonebook_id format: %v", err) }

	// Verify user owns the phonebook
	_, err = s.app.GetPhonebook(ctx, phonebookID, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "phonebook not found or access denied, cannot list contacts")
		}
		return nil, status.Errorf(codes.Internal, "failed to verify phonebook ownership: %v", err)
	}

	domainContacts, totalCount, err := s.app.ListContacts(ctx, phonebookID, int(req.GetOffset()), int(req.GetLimit()))
	if err != nil {
		s.logger.ErrorContext(ctx, "ListContacts app error", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to list contacts: %v", err)
	}
	protoContacts := make([]*pb.Contact, len(domainContacts))
	for i, dc := range domainContacts {
		protoContacts[i] = toProtoContact(dc)
	}
	return &pb.ListContactsResponse{Contacts: protoContacts, TotalCount: int32(totalCount)}, nil
}

func (s *GRPCServer) UpdateContact(ctx context.Context, req *pb.UpdateContactRequest) (*pb.UpdateContactResponse, error) {
	userID, err := getUserIDFromCtx(ctx)
	if err != nil { return nil, err }
    if userID == uuid.Nil { return nil, status.Errorf(codes.Unauthenticated, "user authentication required") }

	contactID, err := uuid.Parse(req.GetId())
	if err != nil { return nil, status.Errorf(codes.InvalidArgument, "invalid contact ID format: %v", err) }
	phonebookID, err := uuid.Parse(req.GetPhonebookId())
	if err != nil { return nil, status.Errorf(codes.InvalidArgument, "invalid phonebook_id format: %v", err) }

	// Verify user owns the phonebook
	_, err = s.app.GetPhonebook(ctx, phonebookID, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "phonebook not found or access denied, cannot update contact")
		}
		return nil, status.Errorf(codes.Internal, "failed to verify phonebook ownership: %v", err)
	}

	domainContact, err := s.app.UpdateContact(ctx, contactID, phonebookID, req.GetNumber(), req.GetFirstName(), req.GetLastName(), req.GetEmail(), req.GetCustomFields(), req.GetSubscribed())
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "contact not found for update")
		}
		if errors.Is(err, domain.ErrDuplicateEntry) {
			return nil, status.Errorf(codes.AlreadyExists, "contact with this number already exists in the phonebook")
		}
		s.logger.ErrorContext(ctx, "UpdateContact app error", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to update contact: %v", err)
	}
	return &pb.UpdateContactResponse{Contact: toProtoContact(domainContact)}, nil
}

func (s *GRPCServer) DeleteContact(ctx context.Context, req *pb.DeleteContactRequest) (*emptypb.Empty, error) {
	userID, err := getUserIDFromCtx(ctx)
	if err != nil { return nil, err }
    if userID == uuid.Nil { return nil, status.Errorf(codes.Unauthenticated, "user authentication required") }

	contactID, err := uuid.Parse(req.GetId())
	if err != nil { return nil, status.Errorf(codes.InvalidArgument, "invalid contact ID format: %v", err) }
	phonebookID, err := uuid.Parse(req.GetPhonebookId())
	if err != nil { return nil, status.Errorf(codes.InvalidArgument, "invalid phonebook_id format: %v", err) }

	// Verify user owns the phonebook
	_, err = s.app.GetPhonebook(ctx, phonebookID, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "phonebook not found or access denied, cannot delete contact")
		}
		return nil, status.Errorf(codes.Internal, "failed to verify phonebook ownership: %v", err)
	}

	err = s.app.DeleteContact(ctx, contactID, phonebookID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "contact not found for delete")
		}
		s.logger.ErrorContext(ctx, "DeleteContact app error", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to delete contact: %v", err)
	}
	return &emptypb.Empty{}, nil
}


// --- Helper functions for type conversion ---

func toProtoPhonebook(pb *domain.Phonebook) *pb.Phonebook {
	if pb == nil {
		return nil
	}
	return &pb.Phonebook{
		Id:          pb.ID.String(),
		UserId:      pb.UserID.String(),
		Name:        pb.Name,
		Description: pb.Description,
		CreatedAt:   timestamppb.New(pb.CreatedAt),
		UpdatedAt:   timestamppb.New(pb.UpdatedAt),
	}
}

func toProtoContact(ct *domain.Contact) *pb.Contact {
	if ct == nil {
		return nil
	}
	customFields := ct.CustomFields
	if customFields == nil { // Ensure map is not nil for proto
		customFields = make(map[string]string)
	}
	return &pb.Contact{
		Id:           ct.ID.String(),
		PhonebookId:  ct.PhonebookID.String(),
		Number:       ct.Number,
		FirstName:    ct.FirstName,
		LastName:     ct.LastName,
		Email:        ct.Email,
		CustomFields: customFields,
		Subscribed:   ct.Subscribed,
		CreatedAt:    timestamppb.New(ct.CreatedAt),
		UpdatedAt:    timestamppb.New(ct.UpdatedAt),
	}
}
