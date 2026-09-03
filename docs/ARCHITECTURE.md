# Architecture

Arcade Vault is a domain API for selling unique arcade cabinets: catalog, atomic stock reservation, orders, and Stripe sandbox payments. The public edge is GraphQL; services talk gRPC; side effects go through RabbitMQ. Traces are the proof the pieces are actually connected.

For the build sequence, see [ROADMAP.md](./ROADMAP.md).

## Why this shape

A cabinet is not a generic SKU. Two customers racing for the last Donkey Kong is the interesting problem. The architecture exists to make that race correct, observable, and boring to operate.

- **GraphQL only on the gateway** — one public schema for humans and demos. Services do not expose GraphQL.
- **gRPC internally** — typed contracts, deadlines, and metadata (`X-User-Id`, trace context).
- **Two bounded contexts, not five** — catalog owns product, stock, and orders. payments owns Stripe and webhook idempotency.
- **sqlc + pgx** — the critical path is SQL (`WHERE qty_available > 0`). An ORM would hide the query that matters and still need raw SQL there.
- **Transactional outbox** — `order.created` is inserted in the same Postgres transaction as the reservation. No publish-then-commit.

## Context

```
                    ┌─────────────────────────────────┐
                    │         graphql-gateway         │
                    │     gqlgen · OTel · slog        │
                    └──────────────┬──────────────────┘
                                   │ gRPC
                     ┌─────────────┴─────────────┐
                     ▼                           ▼
            ┌────────────────┐          ┌────────────────┐
            │    catalog     │          │    payments    │
            │ cabinets, stock│          │ Stripe intents │
            │ orders, outbox │          │ webhooks       │
            └───────┬────────┘          └───────┬────────┘
                    │                           │
                    └──────── RabbitMQ ─────────┘
                              PostgreSQL (sqlc/pgx)
                              Jaeger · slog
```

Postgres in v1 is **one cluster, one schema per service** (`catalog`, `payments`). That signals isolation without running two engines locally. A real database-per-service split is a post-v1 move.

## Checkout sequence

This is the only flow the system has to get right.

```
Client (GraphQL createOrder)
  → gateway (gRPC CreateOrder)
    → catalog, single transaction:
         validate prices
         reserve stock
         insert order + items (pending_payment)
         insert outbox order.created
    → commit
  → outbox publisher → RabbitMQ
  → payments: create Stripe PaymentIntent
  → Stripe webhook
       payment_intent.succeeded → payment.succeeded → catalog marks paid
       payment_intent.payment_failed → payment.failed → catalog fails order, releases stock
```

Stock is reserved at order creation, not at payment success. Payment failure must release it. Both transitions are idempotent.

## Service boundaries

### graphql-gateway

Public BFF. No business rules, no database.

- Translates GraphQL operations to gRPC calls.
- Injects request-id and `X-User-Id` into gRPC metadata.
- Maps domain errors to GraphQL errors (insufficient stock is not an internal error).
- Owns the only GraphQL schema (`api/graphql`).

### catalog

Rich domain. Source of truth for cabinets, availability, and orders.

| Owns | Does not own |
|---|---|
| Cabinets, stock, reservations | Stripe customer or card data |
| Orders and order items | PaymentIntent lifecycle |
| Outbox for `order.created` | Webhook HTTP |
| Consumers for `payment.*` | GraphQL |

**Invariant:** two concurrent `ReserveStock` / `CreateOrder` calls for the last unit of a cabinet yield exactly one success.

### payments

Thin by design. Isolated so Stripe secrets, webhook verification, and provider retries do not leak into catalog.

| Owns | Does not own |
|---|---|
| Payment records | Stock |
| Stripe PaymentIntents | Order status (it *signals*; catalog applies) |
| Webhook signature + `event.id` dedup | Cabinet catalog |
| Events `payment.succeeded` / `payment.failed` | GraphQL |

The webhook endpoint is **HTTP**, not gRPC. Stripe cannot call gRPC.

## Clean Architecture (per service)

```
cmd/<service>/main.go          wiring only
internal/<service>/
  domain/                      entities, value objects, errors — no infra imports
  app/                         use cases + ports (interfaces)
  adapter/
    postgres/                  sqlc, pgx, mappers
    grpc/                      protobuf handlers
    rabbit/                    publish / consume
    stripe/                    payments only
    http/                      payments webhook only
```

Rules:

- `domain` does not import pgx, amqp, stripe-go, otel, or grpc.
- Ports live in `app`, not in `domain`. Domain stays language + business rules.
- sqlc structs are not domain entities. Adapters map both ways.
- `main` constructs adapters and passes them into use cases. No Fx/Wire in v1 — the dependency graph should be readable in one file.

### Domain errors (catalog)

`ErrNotFound`, `ErrInsufficientStock`, `ErrDuplicateReservation`, plus order-level equivalents. gRPC maps them to codes (`NotFound`, `FailedPrecondition`, `AlreadyExists`). The gateway maps those to GraphQL extensions. `fmt.Errorf("db: %w")` stays in adapters.

### Money

Integer cents (`int64`). No `float64` prices on the wire or in Postgres.

## Data

### Access

- Migrations: golang-migrate, one tree per service (`db/catalog`, `db/payments`).
- Queries: sqlc against those schemas.
- Driver: pgx/v5. Transactions via `pgx.Tx` for `CreateOrder` (reserve + order + outbox).

Typical reservation (sketch — the real SQL lives in sqlc):

```sql
UPDATE cabinets
SET qty_available = qty_available - @qty
WHERE id = @id AND qty_available >= @qty
RETURNING *;
```

Zero rows → `ErrInsufficientStock`. That statement is the product.

### Outbox

`CreateOrder` writes `outbox` in the **same** transaction as stock and the order. A publisher polls unpublished rows, publishes to RabbitMQ, then sets `published_at`. At-least-once delivery; consumers must be idempotent.

Never: publish to RabbitMQ and then commit Postgres. A crash between those steps loses the event or the reservation.

### Idempotency

| Surface | Key | Behavior |
|---|---|---|
| `ReserveStock` / `CreateOrder` | client `idempotency_key` | Same key returns the original order, no second reservation |
| Stripe webhook | `event.id` in `processed_stripe_events` | Duplicate POST returns 200, no state change |
| `payment.failed` stock release | order id + current status | Release only from `pending_payment` |

## Messaging

- Exchange: topic `arcadevault`.
- Routing keys: `arcadevault.<aggregate>.<event>.v1`.
- v1 events: `order.created`, `payment.succeeded`, `payment.failed`.
- Payload: `event_id`, `order_id`, `occurred_at`, `amount_cents` (and whatever the consumer needs; keep it small).

catalog produces `order.created` and consumes payment results. payments consumes `order.created` and produces payment results. No cycles beyond that pair.

## Observability

Three signals, in this priority: **traces, logs, then metrics**.

- Each process sets `service.name` (`catalog`, `payments`, `gateway`).
- W3C `traceparent` on gRPC so gateway → catalog → payments stay one trace. Stripe webhooks often start a new root; join them with span attributes `order_id`, `cabinet_id`, and `payment_id` (and the Stripe `event.id`).
- slog JSON; a handler copies `trace_id` / `span_id` from `ctx` onto every record. Stable attrs (`cabinet_id`, `order_id`), not interpolated messages as the only structure.
- Named spans around use cases and I/O: `CreateOrder`, `ReserveCabinet`, `PublishOutbox`, `Stripe.CreatePaymentIntent`, `Stripe.Webhook`.

The demo is: create an order in GraphiQL, paste the trace id (or search service `gateway`) in Jaeger, and walk gateway → catalog → SQL → publish → payments → Stripe → consume.

## Auth (v1)

No OAuth. The gateway reads `X-User-Id` and forwards it as gRPC metadata. Catalog stores that as `orders.user_id`. Fine for a portfolio and for local demos; not fine for production. JWT on the gateway is a post-v1 item.

## Testing strategy

Correctness is proven where the database is real.

- **Unit:** domain and use cases with fake ports (idempotency, release stock).
- **Integration:** testcontainers Postgres (and RabbitMQ when the publisher is real). The stock race test is mandatory.
- **Webhook:** fixture payloads + signature tests; do not hit Stripe’s network in CI for business logic.
- **E2E (optional):** one Compose path with stripe-mock, Phase 6.

## What this architecture deliberately is not

- GraphQL on every service or Apollo federation.
- Kafka, event sourcing, or CQRS with separate read models.
- GORM / AutoMigrate.
- A service mesh or Kubernetes manifest set for v1.
- Inventory split from catalog (a third service) before the race test exists.

Those are valid in other systems. Here they would add surface without making the cabinet race clearer.
