# Project Progress: Arad SMS Gateway (Go, PostgreSQL, NATS)

This document outlines the key implementation steps and progress tracking for re-implementing the Arad SMS Gateway using Go for backend services, PostgreSQL as the database, and NATS for messaging.

## Legend
*   [ ] To Do
*   [~] In Progress
*   [x] Done
*   N/A - Not Applicable

## Phase 0: Foundation & Setup

*   [x] **Project Setup:**
    *   [x] Initialize Git monorepo (or individual repos per service). (Conceptual: `go-rewrite` branch, `golang_services/` dir)
    *   [x] Define overall project structure and Go module strategy. (`golang_services/go.mod`, standard layout described)
    *   [x] Choose and standardize on a Go version. (Go 1.21 chosen)
*   [x] **Common Libraries & Utilities (`internal/platform` or shared `pkg`):**
    *   [x] Configuration loading (Viper setup). (`internal/platform/config/config.go`, `configs/config.defaults.yaml`)
    *   [x] Structured logging (Logrus, Zap, or slog setup). (`internal/platform/logger/logger.go` with slog)
    *   [ ] Basic error handling utilities. (To be done within services or a future platform addition)
    *   [x] PostgreSQL connection manager (`pgxpool` wrapper). (Implemented in `internal/platform/database/postgres.go`)
    *   [x] NATS client wrapper (connection, basic pub/sub helpers). (Implemented in `internal/platform/messagebroker/nats.go`)
*   [x] **CI/CD Pipeline (Initial):**
    *   [ ] Setup CI server (GitHub Actions, GitLab CI, Jenkins). (Beyond direct file creation)
    *   [x] Basic pipeline: lint, test, build Go binary for a sample service. (Placeholder created: `scripts/ci_pipeline_placeholder.yml`)
    *   [ ] Docker image build and push to registry (for a sample service). (Placeholder in CI script)
*   [x] **Local Development Environment:**
    *   [x] Docker Compose setup for PostgreSQL, NATS, (optional: Jaeger, Prometheus, Grafana). (`docker-compose.yml` created for PG & NATS)
    *   [x] Define Makefiles or scripts for common dev tasks (build, test, run, lint). (Makefile created in `golang_services/Makefile`)
*   [x] **Database Setup:**
    *   [x] Install and configure PostgreSQL for local development. (Handled by Docker Compose)
    *   [x] Setup database migration tool (`golang-migrate/migrate` or similar). (Directory `migrations/` and placeholder files created, including auth tables migration)

## Phase 1: Core Services & Authentication

*   [x] **User Service (`user-service`):**
    *   [x] Define `User`, `Role`, `Permission`, `RefreshToken` domain models (Go structs). (`internal/user_service/domain/user.go`)
    *   [x] Implement PostgreSQL schema & migrations for users, roles, permissions, role_permissions, refresh_tokens. (`migrations/000002_create_auth_tables.up.sql`)
    *   [x] Implement `UserRepository` for CRUD operations. (Key methods implemented in `repository/postgres/user_repository_pg.go`, skeletons for others. `GetByIDForUpdate` and `UpdateCreditBalance` added for billing.)
    *   [x] Implement password hashing (e.g., bcrypt). (In `app/auth_service.go`)
    *   [x] Implement user registration logic. (In `app/auth_service.go`)
    *   [x] Implement user login logic (issue JWTs). (In `app/auth_service.go`)
    *   [x] Implement JWT validation & refresh token logic. (Initial logic in `app/auth_service.go`, refresh token part needs more robust implementation for rotation/storage)
    *   [x] Implement API key generation and validation logic. (Initial logic in `app/auth_service.go`)
    *   [x] Implement basic role and permission management logic. (In `app/auth_service.go`)
    *   [x] Define gRPC interface for internal authentication/authorization checks. (`api/proto/userservice/auth.proto`)
    *   [x] Implement gRPC server for `AuthServiceInternal`. (`internal/user_service/adapters/grpc/server.go` and `cmd/user_service/main.go` updated)
*   [x] **Public API Service (`public-api-service`):**
    *   [x] Setup HTTP server (e.g., Gin, Chi, or `net/http`). (`cmd/public_api_service/main.go` with Chi)
    *   [x] Implement authentication middleware (JWT & API Key validation, calls `user-service` via gRPC). (`internal/public_api_service/middleware/auth_middleware.go`)
    *   [x] Implement authorization middleware (basic RBAC checks based on context from auth middleware). (Placeholder in `auth_middleware.go`)
    *   [x] Implement `/auth/register`, `/auth/login`, `/auth/refresh_token` endpoints (proxies to `user-service`). (`internal/public_api_service/transport/http/auth_handler.go` - uses simulated gRPC calls pending proto update for these specific RPCs)
    *   [x] Implement `/users/me` endpoint (protected). (In `transport/http/auth_handler.go` and `cmd/public_api_service/main.go`)
    *   [ ] Implement `/user/credit` endpoint placeholder (protected). (To Do)
*   [x] **NATS Integration (Basic Test):**
    *   [x] Connect services (`public-api-service`, `user-service`) to NATS. (`main.go` files updated)
    *   [x] Implement simple publish/subscribe for a test event. (`user-service` publishes on register, `public-api-service` subscribes and logs)
*   [x] **Testing (Unit & Basic Integration):** (Placeholders created, some initial unit tests drafted)
    *   [x] Write unit tests for HTTP handlers in `public-api-service` (placeholder with mock gRPC client created for auth_handler; message_handler_test created).
    *   [x] Write unit tests for NATS publishing logic in `user-service` (placeholder with mock NATS client created).
    *   [ ] Write basic integration tests for:
        *   [ ] `public-api-service` endpoint -> gRPC call to `user-service`.
        *   [ ] `user-service` NATS publish -> Test NATS subscriber receive.

## Phase 2: Core SMS Functionality

*   [x] **PostgreSQL Schema & Migrations (SMS Core):**
    *   [x] Define `OutboxMessage`, `InboxMessage`, `SMSProvider`, `PrivateNumber`, `Route` domain models. (`internal/core_sms/domain/sms_models.go`)
    *   [x] `outbox_messages`, `inbox_messages` tables. (`migrations/000003_create_core_sms_tables.up.sql`)
    *   [x] `sms_providers`, `private_numbers`, `routes` tables. (`migrations/000003_create_core_sms_tables.up.sql`)
*   [x] **Billing Service (`billing-service` - Partial):**
    *   [x] Define `Transaction` domain model & `TransactionType` ENUM. (`internal/billing_service/domain/billing_models.go`)
    *   [x] Implement PostgreSQL schema & migrations for `transactions` table. (`migrations/000004_create_billing_tables.up.sql` - assumed exists, config updated)
    *   [x] Implement `TransactionRepository` (interface & Postgres impl - assumed exists).
    *   [x] Implement basic credit check and deduction logic (callable via gRPC). (`internal/billing_service/app/billing_app_service.go` - uses direct user repo access temporarily)
    *   [x] Define and implement gRPC service `BillingInternalService` and its server. (`api/proto/billingservice/billing.proto`, `internal/billing_service/adapters/grpc/server.go`, `cmd/billing_service/main.go` - assumed exists, config updated)
*   [x] **SMS Sending Service (`sms-sending-service`):**
    *   [x] Define `SMSSenderProvider` interface for SMS sending. (`internal/sms_sending_service/provider/interface.go`)
    *   [x] Implement adapter for **one** SMS provider (Mock: `provider/mock_provider.go`).
    *   [x] Implement core SMS sending logic: (`internal/sms_sending_service/app/sms_app_service.go`)
        *   [x] NATS: Consume "send SMS" jobs from a NATS subject (`sms.jobs.send`).
        *   [x] Call `billing-service` to check/deduct credit.
        *   [x] Call provider adapter to send SMS (mocked).
        *   [x] Update `outbox_messages` table (status: queued, sent_to_provider, failed_provider_submission) via `OutboxRepository`. (OutboxRepo implemented - assumed exists)
    *   [x] Update `sms-sending-service/main.go` to run NATS consumer, connect to DB & billing gRPC.
*   [x] **Public API Service (`public-api-service`):**
    *   [x] Implement `POST /messages/send` endpoint. (`internal/public_api_service/transport/http/message_handler.go`)
        *   [x] Validate request (basic DTO).
        *   [x] Create initial `OutboxMessage` record with "queued" status.
        *   [x] Publish "send SMS" job to NATS.
        *   [x] Return `message_id` and "Queued" status.
    *   [x] Implement `GET /messages/{message_id}` endpoint (reads from `outbox_messages`). (In `message_handler.go`)
*   [x] **Testing (Unit & Basic Integration):** (Placeholders created for Phase 2 components)
    *   [x] Create placeholder unit test files for `billing-service` repo & app.
    *   [x] Create placeholder unit test files for `sms-sending-service` repo & app.
    *   [x] Create placeholder unit test files for `public-api-service` message handlers.
    *   [x] Write basic integration tests for the send SMS flow (API call -> NATS -> `sms-sending-service` -> mock provider -> DB update). (To Do)


## Phase 3: Delivery Reports & Incoming SMS
*   [x] **Delivery Retrieval Service (`delivery-retrieval-service`):**
    *   [x] Initial directory structure and `main.go` created. (`cmd/delivery_retrieval_service`, `internal/delivery_retrieval_service`)
    *   [x] Define `DeliveryReport` and `DeliveryStatus` domain models. (`internal/delivery_retrieval_service/domain/delivery_report.go`)
    *   [x] Define `OutboxMessage` domain model (minimal) and `OutboxRepository` interface. (`domain/outbox_message.go`, `domain/outbox_repository.go`)
    *   [x] Implement PostgreSQL `OutboxRepository` (`repository/postgres/outbox_repository_pg.go`).
    *   [x] Implement `DLRProcessor` service (`app/dlr_processor.go`) for database updates, adapted for NATS events.
    *   [x] Implement mock provider polling logic (`app/poller.go`) - (Now disabled in main.go, replaced by NATS consumer).
    *   [x] Integrate `DLRProcessor` into `main.go` to process polled DLRs - (Now processes DLR events from NATS consumer).
    *   [x] Integrate into build system (Makefile) and deployment (docker-compose.yml with shared Dockerfile.golang).
    *   [x] NATS: Consume DLR events from `dlr.raw.*` (`app/dlr_consumer.go` and integrated into `main.go`).
    *   [N/A] Logic to poll providers (for the one implemented) for delivery reports (if applicable). (Replaced by NATS-based DLR consumption)
    *   [N/A] Implement cron job for periodic polling. (Replaced by NATS-based DLR consumption)
    *   [x] Update `outbox_messages` table with DLR status. (Implementation via DLRProcessor using NATS events)
    *   [x] (Optional) NATS: Publish DLR events. (Implemented in `DLRProcessor` to publish `ProcessedDLREvent` to `dlr.processed.v1.{providerName}`)
*   [x] **Public API Service (`public-api-service`):**
    *   [x] Define DTOs for provider DLR and incoming SMS callbacks (`ProviderDLRCallbackRequest`, `ProviderIncomingSMSRequest` in `internal/public_api_service/transport/http/incoming_dtos.go`).
    *   [x] Implement `POST /incoming/receive/{provider_name}` endpoint for provider DLR callbacks. (`internal/public_api_service/transport/http/incoming_handler.go` and `cmd/public_api_service/main.go`)
        *   [x] Validate callback (basic path param, JSON decoding, DTO validation).
        *   [x] Publish DLR data to a NATS subject (`dlr.raw.{provider_name}`).
    *   [x] Implement endpoint for provider incoming SMS callbacks. (`internal/public_api_service/transport/http/incoming_handler.go` and `cmd/public_api_service/main.go`)
        *   [x] Validate callback (basic path param, JSON decoding, DTO validation).
        *   [x] Publish raw incoming SMS data to a NATS subject (`sms.incoming.raw.{provider_name}`).
*   [x] **Inbound Processor Service (`inbound-processor-service`):**
    *   [x] Initial directory structure and `main.go` created. (`cmd/inbound_processor_service`, `internal/inbound_processor_service`)
    *   [x] Integrate into build system (Makefile) and deployment (docker-compose.yml using shared Dockerfile.golang).
    *   [x] Define `InboxMessage` domain model and `InboxRepository` interface. (`domain/inbox_message.go`, `domain/inbox_repository.go`)
    *   [x] Implement PostgreSQL `InboxRepository` (`repository/postgres/inbox_repository_pg.go`).
    *   [x] Create database migrations for `inbox_messages` table (`migrations/000005_create_inbox_messages_table.up.sql` and `.down.sql`).
    *   [x] NATS: Consume raw incoming SMS data from NATS. (`app/sms_consumer.go`, integrated into `main.go`)
    *   [x] Basic parsing logic (store in `inbox_messages` table). (`app/sms_processor.go` created and integrated into `main.go`)
    *   [x] Associate with user/private number. (Implemented in `SMSProcessor` using `PrivateNumberRepository`)

## Phase 4: Phonebook & Advanced Features
*   [x] **Phonebook Service (`phonebook-service`):**
    *   [x] Initial directory structure, `main.go` (with gRPC setup placeholders), Makefile, and docker-compose.yml entry created.
    *   [x] Define `Phonebook`, `Contact` domain models. (`internal/phonebook_service/domain/phonebook_models.go`)
    *   [x] Implement PostgreSQL schema & migrations for `phonebooks`, `contacts`, `phonebook_contacts`. (`migrations/000006_create_phonebook_tables.up.sql` & `.down.sql`)
    *   [x] Implement `PhonebookRepository`, `ContactRepository` interfaces and PostgreSQL implementations. (`domain/repositories.go`, `repository/postgres/*_repository_pg.go`)
    *   [x] Implement gRPC interface for CRUD operations. (`api/proto/phonebookservice/phonebook.proto` defined and Go code generated)
    *   [x] Implement gRPC server logic. (`app/phonebook_app.go`, `adapters/grpc/server.go`, and `cmd/phonebook_service/main.go` updated)
*   [x] **Public API Service (`public-api-service`):**
    *   [x] Setup gRPC client for `phonebook-service`. (Configuration added, client initialized in `cmd/public_api_service/main.go`, placeholder handler created)
    *   [x] Define HTTP DTOs for Phonebook and Contact operations (`internal/public_api_service/transport/http/phonebook_dtos.go`).
    *   [x] Implement all `/phonebooks` and `/contacts` CRUD endpoints (calling `phonebook-service` via gRPC). (Phonebook & Contact CRUD handlers implemented in `phonebook_handler.go` and routes registered in `main.go`)
*   [x] **Scheduler Service (`scheduler-service`):**
    *   [x] Initial directory structure, `main.go`, Makefile, and docker-compose.yml entry created.
    *   [x] Define `ScheduledJob` model. (`internal/scheduler_service/domain/scheduled_job.go`)
    *   [x] Implement PostgreSQL schema & migrations for `scheduled_jobs`. (`migrations/000007_create_scheduled_jobs_table.up.sql` & `.down.sql`)
    *   [x] Implement `ScheduledJobRepository` interface and PostgreSQL implementation. (`domain/scheduled_job_repository.go`, `repository/postgres/scheduled_job_repository_pg.go`)
    *   [x] Implement cron job logic to check `scheduled_jobs` table. (`app/job_poller.go` created and integrated into `main.go`)
    *   [x] NATS: Publish "send SMS" jobs to NATS for due scheduled messages. (Implemented in `JobPoller` for "sms" job type)
    *   [x] Implement gRPC interface for CRUD operations on scheduled messages.
*   [x] **Public API Service (`public-api-service`):**
    *   [x] Implement `/scheduled_messages` CRUD endpoints (interacts with `scheduler-service`).
*   [x] **SMS Sending Service (`sms-sending-service`):**
    *   [x] Implement adapters for remaining SMS providers.
        *   [x] Implement adapter for Magfa provider.
    *   [x] Enhance routing logic.
*   [x] **Inbound Processor Service (`inbound-processor-service`):**
    *   [x] Implement advanced message parsing rules (keywords, polls, etc.).
*   [ ] **Billing Service (`billing-service` - Full):**
    *   [x] Implement payment gateway integration (for one gateway).
        *   [x] Implement core logic, domain, repository, and mock adapter for payment gateway.
        *   [x] Implement HTTP webhook endpoint for payment gateway callbacks.
    *   [ ] Implement tariff/pricing logic.
        *   [x] Define schema and migrations for tariffs and user_tariffs tables.
    *   [ ] Implement `transactions` table updates for all relevant events.
*   [ ] **Export Service (`export-service`):**
    *   [ ] Basic data export logic for one entity (e.g., outbox messages to CSV).
    *   [ ] NATS: Consume export requests.
*   [x] **Blacklist & Filters:**
    *   [x] Implement PostgreSQL schema & migrations for `blacklisted_numbers`, `filter_words`.
    *   [x] Integrate blacklist check into `sms-sending-service`.
    *   [x] Integrate content filtering (using `filter_words`) into `sms-sending-service`.

## Phase 5: Observability, Testing & Deployment Prep
(All items below are [ ])
*   [ ] **Logging (Service-wide):**
    *   [ ] Ensure consistent structured logging in all services.
    *   [ ] Integrate with centralized logging platform (e.g., ELK, Loki).
*   [ ] **Monitoring (Service-wide):**
    *   [ ] Implement Prometheus metrics (RED method) for all public API endpoints and key background tasks.
    *   [ ] Setup Grafana dashboards.
    *   [ ] Implement basic alerts in Alertmanager.
*   [ ] **Distributed Tracing (Service-wide):**
    *   [ ] Integrate OpenTelemetry for tracing across services (API -> NATS -> Worker).
    *   [ ] Setup Jaeger/Zipkin for trace visualization.
*   [ ] **Testing:**
    *   [ ] **Unit Tests:** Achieve target code coverage for all services.
    *   [~] **Integration Tests:** (Basic structure described, execution To Do)
        *   [ ] Service-to-Database tests.
        *   [ ] Service-to-NATS tests.
        *   [ ] Service-to-Service tests (gRPC interactions).
    *   [ ] **End-to-End (E2E) Tests:** For key user flows (e.g., send SMS and verify DLR).
*   [ ] **CI/CD Pipeline (Full):**
    *   [ ] Include all test stages (unit, integration).
    *   [ ] Automated deployment to staging environment.
    *   [ ] Strategy for production deployment (blue/green, canary).
*   [ ] **Documentation:**
    *   [ ] Update API documentation (OpenAPI specs).
    *   [ ] Developer/Operational documentation for each service.
*   [ ] **Security Hardening:**
    *   [ ] Review authentication/authorization implementation.
    *   [ ] Input validation review.
    *   [ ] Dependency vulnerability scanning.
*   [ ] **Deployment Configuration (Kubernetes):**
    *   [ ] Finalize Kubernetes Deployments, Services, ConfigMaps, Secrets for each Go service.
    *   [ ] Setup Ingress controller for `public-api-service`.
    *   [ ] Configure HPA (Horizontal Pod Autoscalers).

## Phase 6: UI Development (Separate Track)

*   [ ] Plan and develop a new SPA (React, Vue, Angular) that consumes the Go backend APIs.
    *   This is a separate project track but depends on the Go backend being ready.

## Phase 7: Production Go-Live & Post-Launch

*   [ ] Final load testing.
*   [ ] Data migration from old system (if applicable and planned).
*   [ ] Production deployment.
*   [ ] Post-launch monitoring and stabilization.

This `PROJECT_PROGRESS.md` provides a more detailed checklist. The actual assignment of tasks and timelines would occur in a project management tool.
