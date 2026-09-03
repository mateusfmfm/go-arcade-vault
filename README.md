# Arcade Vault

Domain API for an arcade cabinet shop: unique machines, atomic stock, orders, and Stripe sandbox payments.

Go services with Clean Architecture, gRPC internally, GraphQL at the edge, PostgreSQL via sqlc/pgx, RabbitMQ, and OpenTelemetry → Jaeger. Built as a portfolio system, not a generic CRUD store.

**Status:** foundation. The architecture and build plan are documented; runtime pieces land by [phase](docs/ROADMAP.md). Commands below are the target developer loop — they are not all wired yet.

## Why this exists

Selling a cabinet is a race on a single unit of stock, plus a payment that may fail and must release that unit. That is enough domain to justify:

- explicit SQL for reservations (sqlc + pgx, not an ORM)
- a transactional outbox instead of publish-then-commit
- traces that cross GraphQL → gRPC → Postgres → RabbitMQ → Stripe

## Architecture

```
┌─────────────────────────────────┐
│        graphql-gateway          │
│      public GraphQL (gqlgen)    │
└──────────────┬──────────────────┘
               │ gRPC
     ┌─────────┴─────────┐
     ▼                   ▼
┌──────────────┐   ┌──────────────┐
│   catalog    │   │   payments   │
│ cabinets     │   │ Stripe       │
│ stock, orders│   │ webhooks     │
└──────┬───────┘   └──────┬───────┘
       └──── RabbitMQ ────┘
              PostgreSQL · Jaeger · slog
```

| Process | Role |
|---|---|
| **graphql-gateway** | Public BFF. No database, no business rules. |
| **catalog** | Cabinets, stock reservations, orders, outbox. |
| **payments** | Stripe PaymentIntents, idempotent webhooks. |

Two bounded contexts on purpose. GraphQL lives only on the gateway. Postgres is one cluster with a schema per service.

[Architecture](docs/ARCHITECTURE.md) covers layers, outbox, idempotency, and error mapping. [Roadmap](docs/ROADMAP.md) is the phase-by-phase build.

## Checkout

```
createOrder (GraphQL)
  → catalog reserves stock + writes order + outbox   (one transaction)
  → RabbitMQ  order.created
  → payments creates a Stripe PaymentIntent
  → webhook
       succeeded → order paid
       failed    → order failed, stock released
```

Stock is taken when the order is created, not when Stripe confirms. Failure must roll availability back. Duplicate webhooks are no-ops.

## Stack

| Layer | Choice | Why |
|---|---|---|
| Public API | GraphQL (gqlgen) | One schema for demos and clients |
| Internal API | gRPC + Protobuf | Typed contracts between services |
| Database | PostgreSQL 16, sqlc, pgx/v5 | The reservation *is* the SQL |
| Migrations | golang-migrate | Explicit schema, no AutoMigrate |
| Events | RabbitMQ | `order.created` / `payment.succeeded` / `payment.failed` |
| Payments | Stripe test mode | Real webhook verification, sandbox cards |
| Logs | `log/slog` JSON | `trace_id` on every line |
| Tracing | OpenTelemetry → Jaeger | One span tree for checkout |
| Tests | testcontainers | Race on the last cabinet against real Postgres |
| Local / CI | Docker Compose, GitHub Actions | Clone, up, test |

**Not in v1:** Kubernetes, Redis, Kafka, OAuth, GORM, a product frontend. GraphiQL is the UI.

Auth is an `X-User-Id` header forwarded as gRPC metadata — enough to attribute orders, not a production identity story.

## Repository layout (target)

```
cmd/catalog            cmd/payments           cmd/gateway
internal/<service>/{domain,app,adapter}
api/proto              api/graphql
db/catalog             db/payments
deploy/compose
```

`domain` packages do not import infrastructure.

## Running locally

Target loop once Phase 0+ is in place:

```bash
make dev          # Compose: Postgres, RabbitMQ, Jaeger, services
make test         # unit + integration (testcontainers)
```

| Surface | URL |
|---|---|
| GraphiQL | to be published with the gateway |
| Jaeger UI | `http://localhost:16686` |
| RabbitMQ UI | `http://localhost:15672` |

**See a trace:** create an order in GraphiQL (or the equivalent gRPC call), open Jaeger, search service `gateway`, open the checkout trace. You should see gateway → catalog → SQL → publish → payments → Stripe → consume.

**Stripe (from Phase 4):** test card `4242 4242 4242 4242` for success; `4000 0000 0000 9995` for failure. Forward webhooks with Stripe CLI as documented when payments ships. Copy `.env.example` to `.env` — never commit secrets.

## Tests that matter

Coverage percentage is not the point. These four are:

1. Two goroutines reserve the last cabinet — one wins.
2. `CreateOrder` writes stock, order, and outbox in one transaction.
3. A duplicate Stripe webhook does not apply twice.
4. A failed payment releases stock.

## Docs

- [Architecture](docs/ARCHITECTURE.md) — boundaries, Clean Architecture, data, messaging, observability
- [Roadmap](docs/ROADMAP.md) — phases, definition of done, what not to build

## License

[MIT](LICENSE)
