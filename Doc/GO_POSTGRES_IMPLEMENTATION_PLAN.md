# Technical Implementation Plan: Arad SMS Gateway (Go & PostgreSQL)

## 1. Introduction

This document outlines a technical plan for re-implementing the Arad SMS Gateway system using Go (Golang) for backend services and PostgreSQL as the primary database. This plan is derived from the analysis of the existing .NET-based system documented in `TECHNICAL_DOCUMENTATION.md`. The goal is to create a modern, scalable, and maintainable microservices-based architecture.

## 2. Guiding Principles

*   **Microservices Architecture:** Decompose the system into smaller, independent Go services.
*   **Stateless Services:** Design services to be stateless where possible, relying on PostgreSQL or message queues for state persistence.
*   **Containerization:** Utilize Docker for packaging services.
*   **Orchestration:** Employ Kubernetes for deployment, scaling, and management.
*   **Asynchronous Communication:** Leverage message queues for inter-service communication for resilience and decoupling where appropriate.
*   **Observability:** Implement comprehensive logging, monitoring, and tracing.

## 3. Module Mapping: .NET to Go Microservices

Based on the existing system's modules, the following Go microservices are proposed:

*   **`public-api-service`**: Main public-facing HTTP API (replaces `Arad.SMS.Gateway.WebApi`).
*   **`user-service`**: Manages users, roles, authentication, and authorization.
*   **`sms-sending-service`**: Core logic for sending SMS, with adapters for different providers (replaces .NET SMS Provider Workers).
*   **`delivery-retrieval-service`**: Fetches and processes SMS delivery reports (replaces `Arad.SMS.Gateway.GetSmsDelivery`).
*   **`inbound-processor-service`**: Handles incoming SMS and parsing (replaces `Arad.SMS.Gateway.MessageParser`).
*   **`scheduler-service`**: Manages scheduled SMS and other cron-based tasks (replaces `Arad.SMS.Gateway.RegularContent` and parts of other scheduled .NET workers).
*   **`phonebook-service`**: Manages phonebooks and contacts.
*   **`billing-service`**: Manages credits, transactions, pricing, and payment gateway integrations (replaces `Arad.SMS.Gateway.GiveBackCredit` and financial aspects).
*   **`export-service`**: Handles asynchronous data exports (replaces `Arad.SMS.Gateway.ExportData`).
*   **(Optional) `notification-service`**: For user and system notifications.
*   **(Cross-Cutting)** Logging, common utilities, and data access patterns will be implemented within each service or shared `pkg/internal` libraries, replacing `Arad.SMS.Gateway.GeneralLibrary` and `SqlLibrary`. The .NET Data Layer (Facade, Business, DAL) logic will be distributed among the relevant Go services.

## 4. Go Service Responsibilities

(Refer to the detailed breakdown in "Step 2: Define Responsibilities for Each Go Service" of the planning phase. Key responsibilities include API exposure, user management, SMS sending/receiving/status, phonebook operations, billing, scheduling, and data export.)

## 5. Public API Design (`public-api-service`)

*   **Protocol:** RESTful HTTP/JSON.
*   **Authentication:** API Keys and/or JWT Bearer tokens.
*   **Key Endpoints (v1):**
    *   Messages: `POST /messages/send`, `POST /messages/send/bulk`, `GET /messages/{message_id}`, `GET /messages`
    *   Incoming SMS: `POST /incoming/receive/{provider_name}` (provider callback)
    *   Phonebooks: `POST /phonebooks`, `GET /phonebooks`, `GET /phonebooks/{id}`, `PUT /phonebooks/{id}`, `DELETE /phonebooks/{id}`
    *   Contacts: `POST /phonebooks/{id}/contacts`, `GET /phonebooks/{id}/contacts`, `PUT /contacts/{id}`, `DELETE /contacts/{id}`
    *   User: `GET /user/credit`, `GET /user/info`
    *   Scheduled Messages: `POST /scheduled_messages`, `GET /scheduled_messages`, `GET /scheduled_messages/{id}`, `PUT /scheduled_messages/{id}`, `DELETE /scheduled_messages/{id}`
*   **Internal Communication:** gRPC may be used for inter-service communication for efficiency.
*   **Error Handling:** Standardized JSON error responses.

(Refer to "Step 3: Design the Public API (Go)" for detailed request/response structures.)

## 6. PostgreSQL Database Schema

*   **Conventions:** `snake_case` naming, `UUID` primary keys, `TIMESTAMPTZ` for timestamps (UTC).
*   **Key Tables:**
    *   `users`, `roles`, `permissions`, `role_permissions`, `refresh_tokens`
    *   `outbox_messages` (with `message_status` ENUM), `inbox_messages`
    *   `phonebooks`, `contacts`, `phonebook_contacts`
    *   `transactions` (with `transaction_type` ENUM), `payment_gateways`
    *   `settings`, `user_settings`
    *   `sms_providers`, `routes`
    *   `private_numbers` (sender IDs)
    *   `scheduled_jobs`
    *   `audit_logs`, `blacklisted_numbers`, `filter_words`
*   **Data Types:** Leverage PostgreSQL specific types like `JSONB`, `UUID`, `ENUM`.
*   **Migrations:** Managed via a tool like `golang-migrate/migrate`.

(Refer to "Step 4: Design PostgreSQL Database Schema" for detailed table structures and definitions.)

## 7. Data Access Strategy in Go

*   **Core:** `database/sql` standard library with `jackc/pgx/v5/pgxpool` driver for PostgreSQL.
*   **Pattern:** Repository pattern for each data entity (e.g., `UserRepository`, `MessageRepository`).
*   **Queries:** Primarily raw SQL for control and performance.
*   **Connection Management:** `pgxpool` for connection pooling.
*   **Transactions:** Standard `database/sql` transaction handling (`BeginTx`, `Commit`, `Rollback`).
*   **Migrations Tool:** `golang-migrate/migrate` or similar.

(Refer to "Step 5: Plan Data Access Strategy in Go" for implementation details.)

## 8. Background Task Management in Go

*   **Concurrency:** Goroutines and channels for concurrent execution.
*   **Worker Pools:** For managing concurrent processing of similar tasks (e.g., SMS sending, DLR processing).
*   **Scheduled Tasks:** `robfig/cron/v3` or `go-co-op/gocron` for cron-like job scheduling within services (e.g., `delivery-retrieval-service`, `scheduler-service`).
*   **Message Queues (Recommended):** NATS/JetStream or RabbitMQ for:
    *   Decoupling `public-api-service` from `sms-sending-service` for send requests.
    *   Handling incoming SMS from `public-api-service` to `inbound-processor-service`.
    *   Inter-service notifications.
*   **Graceful Shutdown:** Handle OS signals (SIGINT, SIGTERM) for proper cleanup.
*   **State Management:** Persist necessary task state in PostgreSQL.

(Refer to "Step 6: Outline Background Task Management in Go" for detailed strategies.)

## 9. SMS Provider Integration in Go

*   **HTTP Client:** Standard `net/http` package with configurable `http.Client`.
*   **Provider Adapters:**
    *   Define a common Go `Provider` interface (e.g., `Send(ctx, req) (*SmsResponse, error)`).
    *   Implement this interface for each SMS provider (Arad, Magfa, etc.) in separate packages.
    *   Adapters handle provider-specific request/response formatting (JSON, XML, form-urlencoded), authentication, and error mapping.
*   **Configuration:** Provider API keys, URLs managed via configuration service.
*   **Error Handling & Retries:** Implement exponential backoff for transient errors; distinguish provider errors from network errors.
*   **Context Propagation:** Use `context.Context` for timeouts and cancellation.
*   **Client-Side Rate Limiting:** Implement if required by providers using `golang.org/x/time/rate`.

(Refer to "Step 7: Plan SMS Provider Integration in Go" for specifics.)

## 10. Configuration Management for Go Services

*   **Hierarchy:** Defaults (code) -> Config files -> Environment variables.
*   **File Format:** YAML recommended.
*   **Library:** `spf13/viper` for loading configuration from files and environment variables.
*   **Structure:** Define Go structs for type-safe configuration access.
*   **Secrets:** API keys, database passwords loaded exclusively from environment variables (injected by Kubernetes Secrets or similar in production).
*   **Validation:** Validate configuration at service startup.

(Refer to "Step 8: Define Configuration Management for Go Services" for details.)

## 11. Logging, Monitoring, and Error Handling

*   **Logging:**
    *   Structured logging (JSON or logfmt) using `sirupsen/logrus`, `uber-go/zap`, or `slog` (Go 1.21+).
    *   Configurable log levels. Contextual logging (Trace IDs, Request IDs).
    *   Output to `stdout`/`stderr` for collection by container orchestrator.
    *   Centralized logging (ELK, Loki).
*   **Monitoring:**
    *   Prometheus (`prometheus/client_golang`) for metrics (latency, traffic, errors, saturation).
    *   Grafana for dashboards. Alertmanager for alerts.
    *   Distributed Tracing: OpenTelemetry (OTel) with Jaeger or Zipkin as backend.
*   **Error Handling:**
    *   Idiomatic Go error returns.
    *   Error wrapping (`fmt.Errorf` with `%w`, or `pkg/errors`).
    *   Custom error types/sentinel errors.
    *   Panic for unrecoverable issues only; recover at service boundaries.

(Refer to "Step 9: Outline Logging, Monitoring, and Error Handling in Go" for detailed approaches.)

## 12. Authentication and Authorization Strategy

*   **Authentication:**
    *   **Public API:** API Keys (header-based) for M2M; JWTs (Bearer token) for user-facing apps (SPA, mobile). JWTs issued by `user-service`.
    *   **Inter-Service:** mTLS (if using a service mesh) or Scoped JWTs (OAuth2 Client Credentials flow).
    *   Implemented via middleware (HTTP) or interceptors (gRPC).
*   **Authorization:**
    *   **RBAC (Role-Based Access Control):** Permissions, Roles, Role-Permission mappings stored in PostgreSQL, managed by `user-service`.
    *   Consider Casbin or Open Policy Agent (OPA) for more complex future needs.
    *   Enforcement at both API gateway (coarse-grained) and individual service (fine-grained) levels.

(Refer to "Step 10: Design Authentication and Authorization Strategy" for specifics.)

## 13. UI Layer Considerations

*   The Go backend will provide a headless API (`public-api-service`).
*   **Recommendation:** A new Single Page Application (SPA) using React, Vue, or Angular should be developed to consume these APIs for a modern user experience.
*   This plan focuses on the backend. The new UI would be a separate project/phase.
*   The Go backend API will be designed to support such a modern UI, including appropriate authentication flows (e.g., OAuth2/OIDC for SPAs).

(Refer to "Step 11: Address UI Layer Considerations" for more discussion.)

## 14. Deployment Strategy

*   **Containerization:** Docker for all Go microservices. Multi-stage Dockerfiles for small, secure images.
*   **Orchestration:** Kubernetes (K8s) for production deployment, scaling, and management.
    *   K8s Deployments, Services, Ingress.
    *   ConfigMaps and Secrets for configuration and sensitive data.
*   **Database:** Managed PostgreSQL service (AWS RDS, Google Cloud SQL, Azure DB for PostgreSQL) recommended for production. Dockerized PostgreSQL for dev/test.
*   **CI/CD Pipeline:**
    *   Git (GitHub/GitLab) -> CI Server (Jenkins, GitLab CI, GitHub Actions).
    *   Automated build, lint, test, containerization, push to registry.
    *   Automated deployment to staging and production environments on K8s.
    *   Database migrations (`golang-migrate/migrate`) integrated into the pipeline.
*   **Observability Infrastructure:** Deploy Prometheus, Grafana, ELK/Loki, Jaeger as part of the K8s cluster or as managed services.

(Refer to "Step 12: Propose a Deployment Strategy" for more details.)

## 15. Next Steps / Phased Implementation (High-Level)

1.  **Foundation:** Setup shared libraries (logging, config, basic DB connection), CI/CD pipeline basics.
2.  **Core Services:** Implement `user-service` (authn/authz basics) and `public-api-service` (basic structure).
3.  **Database:** Finalize PostgreSQL schema for core entities and apply initial migrations.
4.  **Key Functionality:**
    *   Implement `sms-sending-service` with one provider adapter.
    *   Implement `billing-service` for credit checking.
    *   Integrate these with the `public-api-service` for sending a basic SMS.
5.  **Expand:** Incrementally add other services (`delivery-retrieval`, `inbound-processor`, `phonebook`, `scheduler`, `export`) and remaining provider adapters.
6.  **UI Development:** Begin development of the new SPA UI in parallel or subsequently.
7.  **Testing:** Thorough unit, integration, and end-to-end testing for each service and the system as a whole.
8.  **Security Hardening:** Penetration testing, security audits.
9.  **Production Deployment & Monitoring:** Go live, closely monitor system performance and stability.

This plan provides a comprehensive roadmap for the redevelopment effort. Each section will require further detailed design and iterative development.
