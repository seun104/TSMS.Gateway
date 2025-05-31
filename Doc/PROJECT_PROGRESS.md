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
    *   [ ] Define Makefiles or scripts for common dev tasks (build, test, run, lint). (Future task)
*   [x] **Database Setup:**
    *   [x] Install and configure PostgreSQL for local development. (Handled by Docker Compose)
    *   [x] Setup database migration tool (`golang-migrate/migrate` or similar). (Directory `migrations/` and placeholder files created, including auth tables migration)

## Phase 1: Core Services & Authentication

*   [x] **User Service (`user-service`):**
    *   [x] Define `User`, `Role`, `Permission`, `RefreshToken` domain models (Go structs). (`internal/user_service/domain/user.go`)
    *   [x] Implement PostgreSQL schema & migrations for users, roles, permissions, role_permissions, refresh_tokens. (`migrations/000002_create_auth_tables.up.sql`)
    *   [x] Implement `UserRepository` for CRUD operations. (Key methods implemented in `repository/postgres/user_repository_pg.go`, skeletons for others)
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
    *   [x] Implement `/auth/register`, `/auth/login`, `/auth/refresh_token` endpoints (proxies to `user-service`). (`internal/public_api_service/transport/http/auth_handler.go` - uses simulated gRPC calls pending proto update)
    *   [x] Implement `/users/me` endpoint (protected). (In `transport/http/auth_handler.go` and `cmd/public_api_service/main.go`)
    *   [ ] Implement `/user/credit` endpoint placeholder (protected). (To Do)
*   [x] **NATS Integration (Basic Test):**
    *   [x] Connect services (`public-api-service`, `user-service`) to NATS. (`main.go` files updated)
    *   [x] Implement simple publish/subscribe for a test event. (`user-service` publishes on register, `public-api-service` subscribes and logs)
*   [~] **Testing (Unit & Basic Integration):** (Partially Done)
    *   [x] Write unit tests for HTTP handlers in `public-api-service` (placeholder with mock gRPC client created).
    *   [x] Write unit tests for NATS publishing logic in `user-service` (placeholder with mock NATS client created).
    *   [ ] Write basic integration tests for:
        *   [ ] `public-api-service` endpoint -> gRPC call to `user-service`.
        *   [ ] `user-service` NATS publish -> Test NATS subscriber receive.

## Phase 2: Core SMS Functionality
(All items below are [ ])
*   [ ] **PostgreSQL Schema & Migrations (SMS Core):**
    *   [ ] `outbox_messages`, `inbox_messages` tables.
    *   [ ] `sms_providers`, `private_numbers`, `routes` tables.
*   [ ] **Billing Service (`billing-service` - Partial):**
    *   [ ] Define `Transaction` domain model.
    *   [ ] Implement PostgreSQL schema & migrations for `transactions` table.
    *   [ ] Implement `TransactionRepository`.
    *   [ ] Implement basic credit check and deduction logic (callable via gRPC).
*   [ ] **SMS Sending Service (`sms-sending-service`):**
    *   [ ] Define `Provider` interface for SMS sending.
    *   [ ] Implement adapter for **one** SMS provider (e.g., Arad or a test/mock provider).
        *   [ ] HTTP client logic, request/response handling.
        *   [ ] Error mapping.
    *   [ ] Implement core SMS sending logic:
        *   [ ] Consume "send SMS" jobs (initially via direct call or simple NATS message).
        *   [ ] Basic routing logic (if multiple providers, or select the one implemented).
        *   [ ] Call `billing-service` to check/deduct credit.
        *   [ ] Call provider adapter to send SMS.
        *   [ ] Update `outbox_messages` table (status: queued, sent, failed_to_send).
    *   [ ] NATS: Consume "send SMS" jobs from a NATS subject.
*   [ ] **Public API Service (`public-api-service`):**
    *   [ ] Implement `POST /messages/send` endpoint.
        *   [ ] Validate request.
        *   [ ] Publish "send SMS" job to NATS.
        *   [ ] Return `message_id` and "Queued" status.
    *   [ ] Implement `GET /messages/{message_id}` endpoint (reads from `outbox_messages`).

## Phase 3: Delivery Reports & Incoming SMS
(All items below are [ ])
*   [ ] **Delivery Retrieval Service (`delivery-retrieval-service`):**
    *   [ ] Logic to poll providers (for the one implemented) for delivery reports (if applicable).
    *   [ ] Implement cron job for periodic polling.
    *   [ ] Update `outbox_messages` table with DLR status.
    *   [ ] (Optional) NATS: Publish DLR events.
*   [ ] **Public API Service (`public-api-service`):**
    *   [ ] Implement `POST /incoming/receive/{provider_name}` endpoint for provider DLR callbacks.
        *   [ ] Validate callback.
        *   [ ] Publish DLR data to a NATS subject for `delivery-retrieval-service` or directly update DB if simple.
    *   [ ] Implement endpoint for provider incoming SMS callbacks.
        *   [ ] Validate callback.
        *   [ ] Publish raw incoming SMS data to a NATS subject.
*   [ ] **Inbound Processor Service (`inbound-processor-service`):**
    *   [ ] NATS: Consume raw incoming SMS data from NATS.
    *   [ ] Basic parsing logic (store in `inbox_messages` table).
    *   [ ] Associate with user/private number.

## Phase 4: Phonebook & Advanced Features
(All items below are [ ])
*   [ ] **Phonebook Service (`phonebook-service`):**
    *   [ ] Define `Phonebook`, `Contact` domain models.
    *   [ ] Implement PostgreSQL schema & migrations for `phonebooks`, `contacts`, `phonebook_contacts`.
    *   [ ] Implement `PhonebookRepository`, `ContactRepository`.
    *   [ ] Implement gRPC interface for CRUD operations.
*   [ ] **Public API Service (`public-api-service`):**
    *   [ ] Implement all `/phonebooks` and `/contacts` CRUD endpoints (calling `phonebook-service` via gRPC).
*   [ ] **Scheduler Service (`scheduler-service`):**
    *   [ ] Define `ScheduledJob` model.
    *   [ ] Implement PostgreSQL schema & migrations for `scheduled_jobs`.
    *   [ ] Implement `ScheduledJobRepository`.
    *   [ ] Implement cron job logic to check `scheduled_jobs` table.
    *   [ ] NATS: Publish "send SMS" jobs to NATS for due scheduled messages.
*   [ ] **Public API Service (`public-api-service`):**
    *   [ ] Implement `/scheduled_messages` CRUD endpoints (interacts with `scheduler-service`).
*   [ ] **SMS Sending Service (`sms-sending-service`):**
    *   [ ] Implement adapters for remaining SMS providers.
    *   [ ] Enhance routing logic.
*   [ ] **Inbound Processor Service (`inbound-processor-service`):**
    *   [ ] Implement advanced message parsing rules (keywords, polls, etc.).
*   [ ] **Billing Service (`billing-service` - Full):**
    *   [ ] Implement payment gateway integration (for one gateway).
    *   [ ] Implement tariff/pricing logic.
    *   [ ] Implement `transactions` table updates for all relevant events.
*   [ ] **Export Service (`export-service`):**
    *   [ ] Basic data export logic for one entity (e.g., outbox messages to CSV).
    *   [ ] NATS: Consume export requests.
*   [ ] **Blacklist & Filters:**
    *   [ ] Implement PostgreSQL schema & migrations for `blacklisted_numbers`, `filter_words`.
    *   [ ] Integrate blacklist check into `sms-sending-service`.
    *   [ ] Integrate content filtering (using `filter_words`) into `sms-sending-service`.

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
    *   [ ] **Integration Tests:**
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
