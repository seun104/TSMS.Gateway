# Project Progress: Arad SMS Gateway (Go, PostgreSQL, NATS)

This document outlines the key implementation steps and progress tracking for re-implementing the Arad SMS Gateway using Go for backend services, PostgreSQL as the database, and NATS for messaging.

## Legend
*   [ ] To Do
*   [~] In Progress
*   [x] Done
*   N/A - Not Applicable

## Phase 0: Foundation & Setup

*   [ ] **Project Setup:**
    *   [ ] Initialize Git monorepo (or individual repos per service).
    *   [ ] Define overall project structure and Go module strategy.
    *   [ ] Choose and standardize on a Go version.
*   [ ] **Common Libraries & Utilities (`internal/platform` or shared `pkg`):**
    *   [ ] Configuration loading (Viper setup).
    *   [ ] Structured logging (Logrus, Zap, or slog setup).
    *   [ ] Basic error handling utilities.
    *   [ ] PostgreSQL connection manager (`pgxpool` wrapper).
    *   [ ] NATS client wrapper (connection, basic pub/sub helpers).
*   [ ] **CI/CD Pipeline (Initial):**
    *   [ ] Setup CI server (GitHub Actions, GitLab CI, Jenkins).
    *   [ ] Basic pipeline: lint, test, build Go binary for a sample service.
    *   [ ] Docker image build and push to registry (for a sample service).
*   [ ] **Local Development Environment:**
    *   [ ] Docker Compose setup for PostgreSQL, NATS, (optional: Jaeger, Prometheus, Grafana).
    *   [ ] Define Makefiles or scripts for common dev tasks (build, test, run, lint).
*   [ ] **Database Setup:**
    *   [ ] Install and configure PostgreSQL for local development.
    *   [ ] Setup database migration tool (`golang-migrate/migrate` or similar).

## Phase 1: Core Services & Authentication

*   [ ] **User Service (`user-service`):**
    *   [ ] Define `User`, `Role`, `Permission` domain models (Go structs).
    *   [ ] Implement PostgreSQL schema & migrations for users, roles, permissions, role_permissions, refresh_tokens.
    *   [ ] Implement `UserRepository` for CRUD operations.
    *   [ ] Implement password hashing (e.g., bcrypt).
    *   [ ] Implement user registration logic.
    *   [ ] Implement user login logic (issue JWTs).
    *   [ ] Implement JWT validation & refresh token logic.
    *   [ ] Implement API key generation and validation logic.
    *   [ ] Implement basic role and permission management logic.
    *   [ ] Define gRPC interface for internal authentication/authorization checks.
*   [ ] **Public API Service (`public-api-service`):**
    *   [ ] Setup HTTP server (e.g., Gin, Chi, or `net/http`).
    *   [ ] Implement authentication middleware (JWT & API Key validation, calls `user-service` via gRPC).
    *   [ ] Implement authorization middleware (basic RBAC checks based on context from auth middleware).
    *   [ ] Implement `/user/login` endpoint (proxies to `user-service`).
    *   [ ] Implement `/user/info` endpoint (protected).
    *   [ ] Implement `/user/credit` endpoint placeholder (protected).
*   [ ] **NATS Integration (Basic):**
    *   [ ] Connect services (`public-api-service`, `user-service`) to NATS.
    *   [ ] Implement simple publish/subscribe for a test event.

## Phase 2: Core SMS Functionality

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
