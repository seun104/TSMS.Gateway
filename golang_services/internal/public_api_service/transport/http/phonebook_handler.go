package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	phonebookPb "github.com/aradsms/golang_services/api/proto/phonebookservice"
	"github.com/aradsms/golang_services/internal/public_api_service/middleware"
)

// PhonebookHandler handles HTTP requests related to phonebooks and contacts.
type PhonebookHandler struct {
	phonebookClient phonebookPb.PhonebookServiceClient
	logger          *slog.Logger
	validate        *validator.Validate
}

// NewPhonebookHandler creates a new PhonebookHandler.
func NewPhonebookHandler(client phonebookPb.PhonebookServiceClient, logger *slog.Logger, validate *validator.Validate) *PhonebookHandler {
	return &PhonebookHandler{
		phonebookClient: client,
		logger:          logger,
		validate:        validate,
	}
}

// Helper to respond with JSON
func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if payload != nil {
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			slog.Default().Error("Failed to write JSON response", "error", err)
		}
	}
}

// Helper to respond with an error
func respondWithError(w http.ResponseWriter, code int, message string) {
	respondWithJSON(w, code, map[string]string{"error": message})
}

// mapGRPCErrorToHTTPStatus converts gRPC status codes to HTTP status codes.
func mapGRPCErrorToHTTPStatus(err error) int {
	s, ok := status.FromError(err)
	if !ok {
		return http.StatusInternalServerError
	}
	switch s.Code() {
	case codes.OK:
		return http.StatusOK
	case codes.Canceled:
		return http.StatusRequestTimeout
	case codes.Unknown, codes.Internal, codes.DataLoss:
		return http.StatusInternalServerError
	case codes.InvalidArgument, codes.FailedPrecondition, codes.OutOfRange:
		return http.StatusBadRequest
	case codes.DeadlineExceeded:
		return http.StatusGatewayTimeout
	case codes.NotFound:
		return http.StatusNotFound
	case codes.AlreadyExists, codes.Aborted:
		return http.StatusConflict
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	case codes.ResourceExhausted:
		return http.StatusTooManyRequests
	case codes.Unimplemented:
		return http.StatusNotImplemented
	case codes.Unavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// RegisterRoutes sets up the routing for phonebook and contact operations.
func (h *PhonebookHandler) RegisterRoutes(r chi.Router) {
	// Phonebook routes
	r.Post("/phonebooks", h.CreatePhonebook)
	r.Get("/phonebooks", h.ListPhonebooks)
	r.Get("/phonebooks/{phonebookID}", h.GetPhonebook)
	r.Put("/phonebooks/{phonebookID}", h.UpdatePhonebook)
	r.Delete("/phonebooks/{phonebookID}", h.DeletePhonebook)

	// Contact routes (nested under phonebooks)
	r.Post("/phonebooks/{phonebookID}/contacts", h.CreateContact)
	r.Get("/phonebooks/{phonebookID}/contacts", h.ListContacts)
	r.Get("/phonebooks/{phonebookID}/contacts/{contactID}", h.GetContact)
	r.Put("/phonebooks/{phonebookID}/contacts/{contactID}", h.UpdateContact)
	r.Delete("/phonebooks/{phonebookID}/contacts/{contactID}", h.DeleteContact)
}

// --- Phonebook Handler Methods ---

func (h *PhonebookHandler) CreatePhonebook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// UserID extraction would happen here via middleware, passed into gRPC context metadata
	// For now, gRPC server side uses a placeholder.

	var reqDTO CreatePhonebookRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&reqDTO); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload: "+err.Error())
		return
	}
	defer r.Body.Close()

	if err := h.validate.StructCtx(ctx, reqDTO); err != nil {
		respondWithError(w, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	grpcRequest := &phonebookPb.CreatePhonebookRequest{
		Name:        reqDTO.Name,
		Description: reqDTO.Description,
	}

	grpcResponse, err := h.phonebookClient.CreatePhonebook(ctx, grpcRequest)
	if err != nil {
		h.logger.ErrorContext(ctx, "gRPC CreatePhonebook call failed", "error", err)
		respondWithError(w, mapGRPCErrorToHTTPStatus(err), "Failed to create phonebook: "+err.Error())
		return
	}
	responseDTO := PhonebookResponseDTO{
		ID:          grpcResponse.Phonebook.GetId(),
		UserID:      grpcResponse.Phonebook.GetUserId(),
		Name:        grpcResponse.Phonebook.GetName(),
		Description: grpcResponse.Phonebook.GetDescription(),
		CreatedAt:   grpcResponse.Phonebook.GetCreatedAt().AsTime(),
		UpdatedAt:   grpcResponse.Phonebook.GetUpdatedAt().AsTime(),
	}
	respondWithJSON(w, http.StatusCreated, responseDTO)
}

func (h *PhonebookHandler) ListPhonebooks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// UserID implicitly handled by gRPC server using context metadata

	offsetStr := r.URL.Query().Get("offset")
	limitStr := r.URL.Query().Get("limit")
	offset, _ := strconv.Atoi(offsetStr)
	limit, _ := strconv.Atoi(limitStr)
	if limit <= 0 { limit = 10 }
	if offset < 0 { offset = 0 }

	grpcRequest := &phonebookPb.ListPhonebooksRequest{Offset: int32(offset), Limit: int32(limit)}
	grpcResponse, err := h.phonebookClient.ListPhonebooks(ctx, grpcRequest)
	if err != nil {
		h.logger.ErrorContext(ctx, "gRPC ListPhonebooks call failed", "error", err)
		respondWithError(w, mapGRPCErrorToHTTPStatus(err), "Failed to list phonebooks: "+err.Error())
		return
	}
	responseDTOs := make([]PhonebookResponseDTO, len(grpcResponse.GetPhonebooks()))
	for i, pb := range grpcResponse.GetPhonebooks() {
		responseDTOs[i] = PhonebookResponseDTO{
			ID: pb.GetId(), UserID: pb.GetUserId(), Name: pb.GetName(), Description: pb.GetDescription(),
			CreatedAt: pb.GetCreatedAt().AsTime(), UpdatedAt: pb.GetUpdatedAt().AsTime(),
		}
	}
	listResponse := ListPhonebooksResponseDTO{
		Phonebooks: responseDTOs, TotalCount: int64(grpcResponse.GetTotalCount()),
		Offset: offset, Limit: limit,
	}
	respondWithJSON(w, http.StatusOK, listResponse)
}

func (h *PhonebookHandler) GetPhonebook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	phonebookIDStr := chi.URLParam(r, "phonebookID")
	if _, err := uuid.Parse(phonebookIDStr); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid phonebook ID format")
		return
	}
	grpcRequest := &phonebookPb.GetPhonebookRequest{Id: phonebookIDStr}
	grpcResponse, err := h.phonebookClient.GetPhonebook(ctx, grpcRequest)
	if err != nil {
		h.logger.ErrorContext(ctx, "gRPC GetPhonebook call failed", "error", err, "phonebook_id", phonebookIDStr)
		respondWithError(w, mapGRPCErrorToHTTPStatus(err), "Failed to get phonebook: "+err.Error())
		return
	}
	responseDTO := PhonebookResponseDTO{
		ID: grpcResponse.Phonebook.GetId(), UserID: grpcResponse.Phonebook.GetUserId(), Name: grpcResponse.Phonebook.GetName(),
		Description: grpcResponse.Phonebook.GetDescription(), CreatedAt: grpcResponse.Phonebook.GetCreatedAt().AsTime(), UpdatedAt: grpcResponse.Phonebook.GetUpdatedAt().AsTime(),
	}
	respondWithJSON(w, http.StatusOK, responseDTO)
}

func (h *PhonebookHandler) UpdatePhonebook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	phonebookIDStr := chi.URLParam(r, "phonebookID")
	if _, err := uuid.Parse(phonebookIDStr); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid phonebook ID format")
		return
	}
	var reqDTO UpdatePhonebookRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&reqDTO); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload: "+err.Error())
		return
	}
	defer r.Body.Close()
	if err := h.validate.StructCtx(ctx, reqDTO); err != nil {
		respondWithError(w, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}
	grpcRequest := &phonebookPb.UpdatePhonebookRequest{Id: phonebookIDStr}
	if reqDTO.Name != nil { grpcRequest.Name = *reqDTO.Name }
	if reqDTO.Description != nil { grpcRequest.Description = *reqDTO.Description }

	grpcResponse, err := h.phonebookClient.UpdatePhonebook(ctx, grpcRequest)
	if err != nil {
		h.logger.ErrorContext(ctx, "gRPC UpdatePhonebook call failed", "error", err, "phonebook_id", phonebookIDStr)
		respondWithError(w, mapGRPCErrorToHTTPStatus(err), "Failed to update phonebook: "+err.Error())
		return
	}
	responseDTO := PhonebookResponseDTO{
		ID: grpcResponse.Phonebook.GetId(), UserID: grpcResponse.Phonebook.GetUserId(), Name: grpcResponse.Phonebook.GetName(),
		Description: grpcResponse.Phonebook.GetDescription(), CreatedAt: grpcResponse.Phonebook.GetCreatedAt().AsTime(), UpdatedAt: grpcResponse.Phonebook.GetUpdatedAt().AsTime(),
	}
	respondWithJSON(w, http.StatusOK, responseDTO)
}

func (h *PhonebookHandler) DeletePhonebook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	phonebookIDStr := chi.URLParam(r, "phonebookID")
	if _, err := uuid.Parse(phonebookIDStr); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid phonebook ID format")
		return
	}
	grpcRequest := &phonebookPb.DeletePhonebookRequest{Id: phonebookIDStr}
	_, err := h.phonebookClient.DeletePhonebook(ctx, grpcRequest)
	if err != nil {
		h.logger.ErrorContext(ctx, "gRPC DeletePhonebook call failed", "error", err, "phonebook_id", phonebookIDStr)
		respondWithError(w, mapGRPCErrorToHTTPStatus(err), "Failed to delete phonebook: "+err.Error())
		return
	}
	respondWithJSON(w, http.StatusNoContent, nil)
}

// --- Contact Handler Methods ---

func (h *PhonebookHandler) CreateContact(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	phonebookIDStr := chi.URLParam(r, "phonebookID")
	if _, err := uuid.Parse(phonebookIDStr); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid phonebook ID format in URL")
		return
	}

	var reqDTO CreateContactRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&reqDTO); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload: "+err.Error())
		return
	}
	defer r.Body.Close()

	if err := h.validate.StructCtx(ctx, reqDTO); err != nil {
		respondWithError(w, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	subscribed := true // Default if not provided or if pointer is nil
	if reqDTO.Subscribed != nil {
		subscribed = *reqDTO.Subscribed
	}

	grpcRequest := &phonebookPb.CreateContactRequest{
		PhonebookId:  phonebookIDStr,
		Number:       reqDTO.Number,
		FirstName:    reqDTO.FirstName,
		LastName:     reqDTO.LastName,
		Email:        reqDTO.Email,
		CustomFields: reqDTO.CustomFields,
		Subscribed:   subscribed,
	}

	grpcResponse, err := h.phonebookClient.CreateContact(ctx, grpcRequest)
	if err != nil {
		h.logger.ErrorContext(ctx, "gRPC CreateContact call failed", "error", err, "phonebook_id", phonebookIDStr)
		respondWithError(w, mapGRPCErrorToHTTPStatus(err), "Failed to create contact: "+err.Error())
		return
	}
	responseDTO := contactToResponseDTO(grpcResponse.GetContact())
	respondWithJSON(w, http.StatusCreated, responseDTO)
}

func (h *PhonebookHandler) ListContacts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	phonebookIDStr := chi.URLParam(r, "phonebookID")
	if _, err := uuid.Parse(phonebookIDStr); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid phonebook ID format in URL")
		return
	}

	offsetStr := r.URL.Query().Get("offset")
	limitStr := r.URL.Query().Get("limit")
	offset, _ := strconv.Atoi(offsetStr)
	limit, _ := strconv.Atoi(limitStr)
	if limit <= 0 { limit = 10 }
	if offset < 0 { offset = 0 }

	grpcRequest := &phonebookPb.ListContactsRequest{
		PhonebookId: phonebookIDStr,
		Offset:      int32(offset),
		Limit:       int32(limit),
	}

	grpcResponse, err := h.phonebookClient.ListContacts(ctx, grpcRequest)
	if err != nil {
		h.logger.ErrorContext(ctx, "gRPC ListContacts call failed", "error", err, "phonebook_id", phonebookIDStr)
		respondWithError(w, mapGRPCErrorToHTTPStatus(err), "Failed to list contacts: "+err.Error())
		return
	}
	responseDTOs := make([]ContactResponseDTO, len(grpcResponse.GetContacts()))
	for i, ct := range grpcResponse.GetContacts() {
		responseDTOs[i] = contactToResponseDTO(ct)
	}
	listResponse := ListContactsResponseDTO{
		Contacts:   responseDTOs, TotalCount: int64(grpcResponse.GetTotalCount()),
		Offset: offset, Limit: limit,
	}
	respondWithJSON(w, http.StatusOK, listResponse)
}

func (h *PhonebookHandler) GetContact(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	phonebookIDStr := chi.URLParam(r, "phonebookID")
	contactIDStr := chi.URLParam(r, "contactID")

	if _, err := uuid.Parse(phonebookIDStr); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid phonebook ID format in URL")
		return
	}
	if _, err := uuid.Parse(contactIDStr); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid contact ID format in URL")
		return
	}

	grpcRequest := &phonebookPb.GetContactRequest{Id: contactIDStr, PhonebookId: phonebookIDStr}
	grpcResponse, err := h.phonebookClient.GetContact(ctx, grpcRequest)
	if err != nil {
		h.logger.ErrorContext(ctx, "gRPC GetContact call failed", "error", err, "phonebook_id", phonebookIDStr, "contact_id", contactIDStr)
		respondWithError(w, mapGRPCErrorToHTTPStatus(err), "Failed to get contact: "+err.Error())
		return
	}
	respondWithJSON(w, http.StatusOK, contactToResponseDTO(grpcResponse.GetContact()))
}

func (h *PhonebookHandler) UpdateContact(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	phonebookIDStr := chi.URLParam(r, "phonebookID")
	contactIDStr := chi.URLParam(r, "contactID")

	if _, err := uuid.Parse(phonebookIDStr); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid phonebook ID format in URL")
		return
	}
	if _, err := uuid.Parse(contactIDStr); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid contact ID format in URL")
		return
	}

	var reqDTO UpdateContactRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&reqDTO); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload: "+err.Error())
		return
	}
	defer r.Body.Close()

	if err := h.validate.StructCtx(ctx, reqDTO); err != nil {
		respondWithError(w, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	grpcRequest := &phonebookPb.UpdateContactRequest{
		Id:           contactIDStr,
		PhonebookId:  phonebookIDStr,
		CustomFields: reqDTO.CustomFields, // Handles nil map correctly if DTO's field is nil
	}
	if reqDTO.Number != nil { grpcRequest.Number = *reqDTO.Number }
	if reqDTO.FirstName != nil { grpcRequest.FirstName = *reqDTO.FirstName }
	if reqDTO.LastName != nil { grpcRequest.LastName = *reqDTO.LastName }
	if reqDTO.Email != nil { grpcRequest.Email = *reqDTO.Email }
	if reqDTO.Subscribed != nil { grpcRequest.Subscribed = *reqDTO.Subscribed }


	grpcResponse, err := h.phonebookClient.UpdateContact(ctx, grpcRequest)
	if err != nil {
		h.logger.ErrorContext(ctx, "gRPC UpdateContact call failed", "error", err, "phonebook_id", phonebookIDStr, "contact_id", contactIDStr)
		respondWithError(w, mapGRPCErrorToHTTPStatus(err), "Failed to update contact: "+err.Error())
		return
	}
	respondWithJSON(w, http.StatusOK, contactToResponseDTO(grpcResponse.GetContact()))
}

func (h *PhonebookHandler) DeleteContact(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	phonebookIDStr := chi.URLParam(r, "phonebookID")
	contactIDStr := chi.URLParam(r, "contactID")

	if _, err := uuid.Parse(phonebookIDStr); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid phonebook ID format in URL")
		return
	}
	if _, err := uuid.Parse(contactIDStr); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid contact ID format in URL")
		return
	}

	grpcRequest := &phonebookPb.DeleteContactRequest{Id: contactIDStr, PhonebookId: phonebookIDStr}
	_, err := h.phonebookClient.DeleteContact(ctx, grpcRequest)
	if err != nil {
		h.logger.ErrorContext(ctx, "gRPC DeleteContact call failed", "error", err, "phonebook_id", phonebookIDStr, "contact_id", contactIDStr)
		respondWithError(w, mapGRPCErrorToHTTPStatus(err), "Failed to delete contact: "+err.Error())
		return
	}
	respondWithJSON(w, http.StatusNoContent, nil)
}

// Helper to convert gRPC Contact to ContactResponseDTO
func contactToResponseDTO(ct *phonebookPb.Contact) ContactResponseDTO {
	if ct == nil {
		return ContactResponseDTO{} // Should ideally not happen if gRPC returns a contact
	}
	customFields := ct.GetCustomFields()
	if customFields == nil {
		customFields = make(map[string]string)
	}
	return ContactResponseDTO{
		ID:           ct.GetId(),
		PhonebookID:  ct.GetPhonebookId(),
		Number:       ct.GetNumber(),
		FirstName:    ct.GetFirstName(),
		LastName:     ct.GetLastName(),
		Email:        ct.GetEmail(),
		CustomFields: customFields,
		Subscribed:   ct.GetSubscribed(),
		CreatedAt:    ct.GetCreatedAt().AsTime(),
		UpdatedAt:    ct.GetUpdatedAt().AsTime(),
	}
}
