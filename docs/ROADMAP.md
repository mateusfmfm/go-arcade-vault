# Roadmap

How Arcade Vault is built. The storefront lives in the [README](../README.md); this file is the build plan.

**Rule:** do not start the next phase until the previous definition of done is green. Each phase is a demonstrable commit (or PR).

## Locked decisions

| Topic | Choice |
|---|---|
| Services | `catalog` + `payments` + `graphql-gateway` |
| Public API | GraphQL (gqlgen) on the gateway only |
| Internal API | gRPC + Protobuf (buf) |
| Database | PostgreSQL 16, one cluster, one schema per service |
| Data access | sqlc + pgx/v5 — not GORM |
| Migrations | golang-migrate |
| Events | RabbitMQ, topic exchange `arcadevault` |
| Consistency | Transactional outbox (same transaction as the stock reservation) |
| Payments | Stripe sandbox + idempotent webhooks |
| Logs | `log/slog` JSON with `trace_id` |
| Tracing | OpenTelemetry → Jaeger |
| Tests | Domain unit tests; integration tests with testcontainers (Postgres + RabbitMQ) |
| Local | Docker Compose |
| CI | GitHub Actions |
| Auth (v1) | `X-User-Id` header (documented; OAuth later) |
| Out of v1 | Kubernetes, Redis, Kafka, a heavy frontend, GraphQL on every service |

## Target layout

Filled in as phases land; created in Phase 0.

```
cmd/catalog/main.go
cmd/payments/main.go
cmd/gateway/main.go

internal/catalog/{domain,app,adapter/{postgres,grpc,rabbit}}
internal/payments/{domain,app,adapter/{postgres,grpc,stripe,rabbit,http}}
internal/gateway/{graph,grpcclient,otel}

api/proto/catalog/v1/*.proto
api/proto/payments/v1/*.proto
api/graphql/schema.graphql

db/catalog/{migrations,queries}
db/payments/{migrations,queries}

deploy/compose/docker-compose.yml
.github/workflows/ci.yml
Makefile
```

`domain` must not import pgx, amqp, stripe, otel, or grpc.

## Event contract (write this into the README in Phase 2)

```
order.created          catalog → payments
payment.succeeded      payments → catalog
payment.failed         payments → catalog
```

Minimum payload: `event_id`, `order_id`, `occurred_at`, `amount_cents`. Version routing keys: `arcadevault.order.created.v1`.

---

## Phase 0 — Foundation (1–2 days)

**Done when:** `docker compose up` starts Postgres, RabbitMQ, and Jaeger. A `catalog` binary answers gRPC health. CI runs `go test ./...`. The README explains how to start the stack and open Jaeger.

1. Short README: product one-liner + mermaid checkout diagram (product contract).
2. `docker-compose.yml`: Postgres 16 with `catalog` and `payments` schemas; RabbitMQ management; Jaeger (OTLP `4317` / `4318`).
3. Makefile: `compose-up`, `test`, `lint`.
4. `cmd/catalog`: gRPC health + OTel interceptor + slog with `trace_id`.
5. GitHub Actions: `go vet`, golangci-lint (or staticcheck), tests.
6. `.env.example` (no secrets committed).

A `grpcurl` against health must show up as a span in Jaeger, and the JSON log must carry the same `trace_id`.

**Do not:** GraphQL, Stripe, a second service, sqlc.

## Phase 1 — Catalog domain + stock (the core)

**Model:** `Cabinet` (id, slug, name, type upright/cocktail, display crt/lcd, condition, price, `qty_available`). Parts can wait.

**Rules:** `qty_available = 0` cannot be reserved; reservation is atomic.

- Initial migration (`cabinets`).
- sqlc: `GetCabinet`, `ListCabinets`, `ReserveCabinet` with `WHERE qty_available > 0` + `RETURNING` (or `SELECT … FOR UPDATE` in a transaction).
- Use case `ReserveStock(ctx, cabinetID, qty, idempotencyKey)`.
- Domain errors: `ErrNotFound`, `ErrInsufficientStock`, `ErrDuplicateReservation`.
- gRPC: `ListCabinets`, `GetCabinet`, `ReserveStock`.
- Seed 3–5 real cabinets (not “Product 1”).

**The test that matters:** testcontainers Postgres, two goroutines reserve the last cabinet. One succeeds, one gets `ErrInsufficientStock`. Without this test the phase is not done.

**Done when:** sqlc generated, use case covered, race test green, the SQL span visible in Jaeger.

**Do not:** orders, the queue, GraphQL.

## Phase 2 — Order + outbox (still catalog only)

**Model**

- `orders` — id, user_id, status (`pending_payment | paid | cancelled | failed`), total.
- `order_items` — cabinet_id, qty, unit_price.
- `outbox` — id, aggregate_id, event_type, payload JSON, published_at.
- `idempotency_keys` — key, order_id.

**Use case `CreateOrder`** — one pgx transaction:

1. Validate items and prices in the database (never trust the client).
2. Reserve stock.
3. Insert order + items.
4. Insert outbox `order.created`.
5. Commit.

Publisher (goroutine or in-process worker): read unpublished outbox rows, publish to RabbitMQ, set `published_at`. If publish fails, the row stays. Never publish-then-commit.

gRPC: `CreateOrder`, `GetOrder`.

**Tests:** CreateOrder reserves and writes the outbox in the same transaction (rollback on invalid items). Publisher coverage can start with a fake publisher + Postgres outbox test; real RabbitMQ in Phase 3/4.

**Done when:** an order is `pending_payment`, an outbox row exists, stock is decremented — all atomically.

## Phase 3 — GraphQL BFF

Minimum schema: `Cabinet`, `Order`, queries `cabinets` / `cabinet` / `order`, mutation `createOrder`.

Gateway: gqlgen, gRPC clients to catalog (payments later), middleware for request-id, OTel, and `X-User-Id` → gRPC metadata. Domain errors mapped (stock → a 409-style GraphQL extension, never a 500).

**Done when:** local GraphiQL lists cabinets, creates an order, and a **single trace** spans gateway → catalog → SQL.

**Do not:** federation, heavy DataLoader, JWT. Add DataLoader only if an N+1 actually hurts.

## Phase 4 — payments + Stripe + events

payments stays thin on purpose.

- `payments` table: id, order_id, stripe_payment_intent_id, status, amount_cents.
- `processed_stripe_events` (event_id PK) — webhook idempotency.

**Flow**

1. RabbitMQ consumer on `order.created` → create a Stripe test PaymentIntent → persist.
2. HTTP `POST /webhooks/stripe` (Stripe is HTTP, not gRPC).
3. Verify signature (`STRIPE_WEBHOOK_SECRET`).
4. Duplicate `event.id` → 200 and stop.
5. `payment_intent.succeeded` → `payment.succeeded`.
6. `payment_intent.payment_failed` / cancel → `payment.failed`.

**catalog consumes**

- `payment.succeeded` → order `paid` (stock was already reserved on create).
- `payment.failed` → order `failed`, **release stock** (symmetric increment, idempotent).

Local: `stripe listen --forward-to …` documented in the README (or stripe-cli in Compose).

**Tests:** duplicate webhook does not double-apply; failed payment releases stock (testcontainers); Stripe payload fixtures (do not call Stripe in logic tests). Optional stripe-mock contract test.

**Done when:** GraphQL creates an order → Jaeger shows gateway, catalog, publish, payments, Stripe (or stub), webhook, consume, update. The order ends `paid` or `failed` with coherent stock.

## Phase 5 — Observability that is actually useful

- Resource attributes: `service.name=catalog|payments|gateway`.
- W3C propagation on gRPC and the webhook HTTP path.
- slog handler injects `trace_id` / `span_id` from every `ctx`.
- Named spans: `CreateOrder`, `ReserveCabinet`, `PublishOutbox`, `Stripe.CreatePaymentIntent`, `Stripe.Webhook`.
- Metrics (optional, cheap): `orders_created`, `reservations_rejected`, `payments_succeeded`. Tracing beats metrics for this portfolio. Skip Prometheus if it delays the rest.

**Done when:** the README can say: open Jaeger, search `gateway`, look at checkout.

## Phase 6 — Portfolio polish

1. README: problem, architecture, how to run, **how to see a trace**, Stripe test cards.
2. This architecture doc kept accurate.
3. Seeds + a demo walkthrough (create an order, point at the trace).
4. CI: proto/sqlc generated and committed, or generated in CI with `git diff --exit-code`.
5. No competing frontend — GraphiQL is the UI, or a one-page order status later.

**Done when:** clone from zero → Compose → GraphiQL → order → Jaeger, without reading source.

---

## Package order inside each service

Every use case, always:

1. `domain` — entities, VOs (`Money` in cents), errors.
2. `app` — ports (interfaces) + use case.
3. `adapter/postgres` — sqlc, domain ↔ row mappers.
4. `adapter/grpc` or `http` — decode, call use case, encode.
5. `adapter/rabbit` — last.
6. `cmd` — wiring. Explicit constructors in `main` beat Fx/Wire for v1.

## Tests worth writing

| Kind | Where | What |
|---|---|---|
| Unit | `domain`, `app` with fake ports | rules, idempotency, stock release |
| Integration | `adapter/postgres` | sqlc + testcontainers + the race |
| Contract | proto (+ optional GraphQL golden) | breaking changes |
| E2E | one Compose test (optional Phase 6) | createOrder → paid with stripe-mock |

Do not chase 90% coverage. Chase: stock race, atomic outbox, idempotent webhook, failed payment releases stock.

## Honest schedule (part-time)

| Phase | Time | Visible delivery |
|---|---|---|
| 0 Foundation | 1–2 d | Compose + health + Jaeger + CI |
| 1 Catalog/stock | 3–5 d | gRPC + race test |
| 2 Order/outbox | 3–4 d | Atomic CreateOrder |
| 3 GraphQL | 2–3 d | GraphiQL end to end |
| 4 Stripe/events | 4–6 d | paid/failed + stock |
| 5 OTel polish | 1–2 d | One complete trace |
| 6 Portfolio | 2–3 d | README + demo |

About **3–4 weeks** at a steady pace. If time is tight, cut in this order: Prometheus → Parts → Compose E2E → any UI.

Never cut: the race test, the outbox, webhook idempotency, the Jaeger walkthrough in the README.

## Git history as storytelling

- `feat: local stack, otel health, ci`
- `feat(catalog): reserve stock with sqlc and race test`
- `feat(catalog): create order with transactional outbox`
- `feat(gateway): graphql catalog and createOrder`
- `feat(payments): stripe intent and webhooks`
- `docs: architecture and demo walkthrough`

## Anti-roadmap

These derail the repo: starting with GraphQL or Stripe; a third microservice in v1; Cognito/Keycloak before stock; GORM “just for now”; event sourcing / heavy CQRS; Kubernetes; a Next.js frontend to look pretty; reopening the library list every week.

## After v1

Only once v1 runs:

1. `Part` + BOM (parts per cabinet).
2. Real auth (JWT) on the gateway.
3. Reservation expiry (TTL + consumer).
4. Schema-per-service → true database-per-service.
5. A hosted deploy (Fly/Render) + remote tracing.
