# GoreFlow agent instructions

GoreFlow is a Go service for durable execution of background jobs and, later, multi-step workflows. Read [docs/project-context.md](docs/project-context.md) before proposing or making architectural changes.

## Project principles

- Keep the system a modular monolith until there is an explicit reason to split it.
- Keep the domain independent of HTTP, PostgreSQL, goroutines, and concrete third-party libraries.
- Put orchestration in the application layer and infrastructure details in adapters.
- PostgreSQL is both the durable store and the job queue. Do not add Kafka or another message broker.
- Design for at-least-once execution; executors must be idempotent.
- Change job state transactionally and enforce transitions explicitly.
- Keep business logic out of HTTP handlers.
- Workers must support graceful shutdown. Later lifecycle stages add leases, heartbeats, retries, cancellation, and crash recovery.
- Do not expand the MVP with DAGs, a visual workflow editor, LLM integrations, microservices, or complex authorization.
- Do not turn roadmap ideas or open questions into implemented requirements without an explicit decision.

## Stack

- Go 1.26.5
- PostgreSQL
- HTTP API
- Docker / Docker Compose for the local environment
- No Kafka or other external message broker

## Working agreement

- The project owner writes the backend implementation. Codex may help with planning, architecture review, API contracts, test scenarios, and frontend work unless asked to implement backend code.
- Prefer a small vertical slice and tests before adding the next lifecycle capability.
- Preserve the distinction in `docs/project-context.md` between accepted decisions, planned work, and unresolved questions.
