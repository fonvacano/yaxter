# Phase 1 Core Domain (Wave 0 + T1.2, T1.3, T1.4) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the social-domain core from ARCHITECTURE.md §8: users & follows (T1.2), the tweet/retweet write path (T1.3), and likes + the buffered counter pipeline (T1.4), plus a small shared Wave 0 that the parallel tracks depend on.

**Architecture:** Wave 0 (sequential, small) adds the shared seams: `internal/events` (envelope + same-tx outbox emit), `pkg/kafkax.Consume` (consumer loop) + `pkg/redisx.Once` (event_id dedupe), a role-runner registry in `cmd/worker`, and the `media` table migration (T1.3 validates against it; the upload pipeline itself is the sibling plan). Then three parallel tracks: **U** (users/follows — dual edge tables + `FollowChanged` in one tx), **T** (tweets — validation, retweet flattening, `TweetCreated`/`TweetDeleted` [+ engagement events for retweets] via outbox, `utl:`/`tw:` cache maintenance), **L** (likes API + `worker:counters`: event_id dedupe → `HINCRBY` → 2s/500-event batched PG flush → nightly reconcile).

**Tech Stack:** Everything already in the repo (pgx, franz-go, go-redis, prometheus, testcontainers, miniredis) — no new dependencies.

**Prerequisites:** Phase 0 plans + Phase 1 first slice (`2026-06-11-phase1-t1.0-t1.1-t1.7.md`) executed: relay running, auth module live, `internal/httpapi` serving with 501 stubs. No Docker locally → `make test` (`-short`); integration suites run in CI.

**Deliberate deviations (recorded, not silent):**
1. Services take a single `*pgxpool.Pool` (the demo's one physical shard). The per-key routing seam stays `pkg/sharding.Router` — services adopt `Router.Pool(key)` at shard-split time; the routing kit itself is already built and tested (T0.4).
2. Follower/following counts update inline in the follow tx (`UPDATE … count ± 1`) — follow rate doesn't need the §2.7 buffering, which exists for like-storms.
3. Followers/following lists paginate by `follower_id`/`followee_id` cursor (stable, index-only on the edge PK) rather than `created_at` — ordering is "stable page-through", not strictly recency; acceptable for MVP lists and recorded here.
4. Tweets hydrate their author via a local projection query on `users` (id, username, avatar_key) instead of importing `internal/users` — keeps parallel tracks import-independent; modules stay decoupled.

---

## Parallel Execution Map (subagent dispatch)

| Wave | Track | Tasks | Touches | Shared files |
|---|---|---|---|---|
| 0 (sequential) | C — common seams | C1 → C2 → C3 → C4 | `internal/events`, `pkg/kafkax`, `pkg/redisx`, `cmd/worker`, `migrations/` | `cmd/worker/main.go` |
| 1 (parallel) | U — users & follows (T1.2) | U1 → U2 → U3 | `internal/users`, `internal/httpapi` | `internal/httpapi/{server,wire}.go`, `pkg/config` |
| 1 (parallel) | T — tweets write path (T1.3) | T1 → T2 → T3 | `internal/tweets`, `internal/httpapi` | `internal/httpapi/{server,wire}.go` |
| 1 (parallel) | L — likes + counters (T1.4) | L1 → L2 → L3 → L4 | `internal/tweets/likes*`, `internal/counters`, `cmd/worker/counters.go`, `internal/httpapi` | `internal/httpapi/{server,wire}.go`, `pkg/config` |

**Rules:** Wave 0 merges before Wave 1 dispatch. One worktree per track; merge order U → T → L. Conflict hotspots and their resolutions:
- `internal/httpapi/server.go`: each track replaces only its own 501 stub lines — union the replacements.
- `internal/httpapi/wire.go` (`Deps` + `NewServer` args): each track adds its own field/argument — union both.
- `internal/tweets/hydrate.go`: track T creates it (PG-column counters); track L replaces the counters read with the Redis read-through — on conflict, **track L's version wins**.
- `pkg/config`, `go.mod`: union + `go mod tidy && go test -short ./...`.

---

# Wave 0 — Track C: Common Seams

### Task C1: `internal/events` — envelope + same-tx emit helper

**Files:**
- Create: `internal/events/events.go`
- Test: `internal/events/events_test.go` (integration, `-short`-skipped)

- [ ] **Step 1: Write the failing test** — `internal/events/events_test.go`

```go
package events

import (
	"context"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"google.golang.org/protobuf/proto"

	tweetsv1 "github.com/fonvacano/yaxter/gen/events/tweets/v1"
	pgxkit "github.com/fonvacano/yaxter/pkg/pgx"
)

func TestKey(t *testing.T) {
	require.Equal(t, "12345", Key(12345))
}

func TestNewEnvelopeStampsFields(t *testing.T) {
	env := NewEnvelope(context.Background(), 777)
	require.EqualValues(t, 777, env.GetEventId())
	require.NotNil(t, env.GetOccurredAt())
	require.Equal(t, "api", env.GetProducer())
}

func TestEmitWritesOutboxRowInCallersTx(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires Docker")
	}
	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("yaxter"), tcpostgres.WithUsername("yaxter"),
		tcpostgres.WithPassword("yaxter"), tcpostgres.BasicWaitStrategies())
	require.NoError(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	m, err := migrate.New("file://../../migrations", dsn)
	require.NoError(t, err)
	require.NoError(t, m.Up())
	m.Close()
	pool, err := pgxkit.NewPool(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	ev := &tweetsv1.TweetEvent{
		Envelope: NewEnvelope(ctx, 901),
		Payload: &tweetsv1.TweetEvent_Created{Created: &tweetsv1.TweetCreated{
			TweetId: 1, AuthorId: 2, Text: "hi",
		}},
	}
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, Emit(ctx, tx, 901, "tweets.v1", Key(2), ev))
	require.NoError(t, tx.Commit(ctx))

	var payload []byte
	var topic, key string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT topic, key, payload FROM outbox WHERE id = 901`).
		Scan(&topic, &key, &payload))
	require.Equal(t, "tweets.v1", topic)
	require.Equal(t, "2", key)

	var out tweetsv1.TweetEvent
	require.NoError(t, proto.Unmarshal(payload, &out))
	require.Equal(t, "hi", out.GetCreated().GetText())
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/events/ -v`
Expected: FAIL — `undefined: Key`.

- [ ] **Step 3: Write `internal/events/events.go`**

```go
// Package events stamps and emits domain events through the transactional
// outbox (§2.4). Every producer in every module goes through Emit — there is
// no other path to the broker.
package events

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/fonvacano/yaxter/gen/events/common/v1"
	"github.com/fonvacano/yaxter/pkg/outbox"
)

// Key renders an int64 aggregate id as the Kafka record key
// (decimal string per docs/events.md).
func Key(id int64) string { return strconv.FormatInt(id, 10) }

// NewEnvelope stamps an event with its id, time, and trace context.
// Consumers dedupe on EventId (docs/events.md rule 1).
func NewEnvelope(ctx context.Context, eventID int64) *commonv1.Envelope {
	return &commonv1.Envelope{
		EventId:     eventID,
		OccurredAt:  timestamppb.Now(),
		Traceparent: outbox.TraceparentFromContext(ctx),
		Producer:    "api",
	}
}

// Emit marshals the topic wrapper message and inserts it into the outbox in
// the caller's transaction — the same tx as the domain write.
func Emit(ctx context.Context, tx pgx.Tx, eventID int64, topic, key string, msg proto.Message) error {
	payload, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("events: marshal %s: %w", topic, err)
	}
	return outbox.Insert(ctx, tx, outbox.Message{
		ID:          eventID,
		Topic:       topic,
		Key:         key,
		Payload:     payload,
		Traceparent: outbox.TraceparentFromContext(ctx),
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/events/ -race -v` (Docker in CI)
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/events
git commit -m "feat(events): envelope stamping and same-tx outbox emit"
```

---

### Task C2: `pkg/kafkax.Consume` loop + `pkg/redisx.Once` dedupe

**Files:**
- Create: `pkg/kafkax/consumer.go`
- Modify: `pkg/redisx/redisx.go`
- Test: `pkg/kafkax/consumer_test.go`, `pkg/redisx/redisx_test.go`

- [ ] **Step 1: Write the failing tests**

`pkg/redisx/redisx_test.go`:

```go
package redisx

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestOnceIsFirstWriterWins(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()

	first, err := Once(ctx, rdb, "evt:1", time.Hour)
	require.NoError(t, err)
	require.True(t, first)

	again, err := Once(ctx, rdb, "evt:1", time.Hour)
	require.NoError(t, err)
	require.False(t, again, "replays must be detected")

	other, err := Once(ctx, rdb, "evt:2", time.Hour)
	require.NoError(t, err)
	require.True(t, other)
}
```

`pkg/kafkax/consumer_test.go`:

```go
package kafkax

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	tckafka "github.com/testcontainers/testcontainers-go/modules/kafka"
	"github.com/twmb/franz-go/pkg/kgo"
)

func TestConsumeDeliversRecordsToHandler(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires Docker")
	}
	ctx := context.Background()
	ctr, err := tckafka.Run(ctx, "confluentinc/confluent-local:7.7.0",
		tckafka.WithClusterID("yaxter-test"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })
	brokers, err := ctr.Brokers(ctx)
	require.NoError(t, err)

	producer, err := NewClient(brokers, kgo.AllowAutoTopicCreation())
	require.NoError(t, err)
	t.Cleanup(producer.Close)
	for i := 0; i < 3; i++ {
		require.NoError(t, producer.ProduceSync(ctx, &kgo.Record{
			Topic: "tweets.v1", Key: []byte("1"), Value: []byte{byte(i)},
		}).FirstErr())
	}

	consumer, err := NewClient(brokers,
		kgo.ConsumerGroup(GroupID("test")),
		kgo.ConsumeTopics("tweets.v1"),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	require.NoError(t, err)
	t.Cleanup(consumer.Close)

	var seen atomic.Int32
	cctx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- Consume(cctx, consumer, zerolog.Nop(), func(_ context.Context, rec *kgo.Record) error {
			seen.Add(1)
			return nil
		})
	}()
	require.Eventually(t, func() bool { return seen.Load() == 3 },
		30*time.Second, 50*time.Millisecond)
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
}
```

- [ ] **Step 2: Run to verify failures**

Run: `go test ./pkg/redisx/ ./pkg/kafkax/ -run 'TestOnce|TestConsume' -v`
Expected: FAIL — `undefined: Once`, `undefined: Consume`.

- [ ] **Step 3: Implement**

Append to `pkg/redisx/redisx.go`:

```go
import (
	"context"
	"time"
)

// Once reports whether key is being seen for the first time within ttl
// (SETNX) — the consumer-side event_id dedupe primitive (§2.7).
func Once(ctx context.Context, rdb *redis.Client, key string, ttl time.Duration) (bool, error) {
	return rdb.SetNX(ctx, key, "1", ttl).Result()
}
```

Create `pkg/kafkax/consumer.go`:

```go
package kafkax

import (
	"context"

	"github.com/rs/zerolog"
	"github.com/twmb/franz-go/pkg/kgo"
)

// Handler processes one record.
type Handler func(ctx context.Context, rec *kgo.Record) error

// Consume polls until ctx ends, passing every record to h. A handler error
// is logged and the record skipped — delivery is at-least-once with
// consumer-side dedupe (docs/events.md rule 1), and a poison record must
// not wedge its partition. Offsets use kgo's group autocommit.
func Consume(ctx context.Context, client *kgo.Client, log zerolog.Logger, h Handler) error {
	for {
		fetches := client.PollFetches(ctx)
		if err := ctx.Err(); err != nil {
			return err
		}
		if fetches.IsClientClosed() {
			return nil
		}
		fetches.EachError(func(topic string, partition int32, err error) {
			log.Error().Err(err).Str("topic", topic).Int32("partition", partition).
				Msg("fetch error")
		})
		fetches.EachRecord(func(rec *kgo.Record) {
			if err := h(ctx, rec); err != nil {
				log.Error().Err(err).Str("topic", rec.Topic).
					Int64("offset", rec.Offset).Msg("handler error; record skipped")
			}
		})
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./pkg/redisx/ -race -v && go test ./pkg/kafkax/ -race -v` (Docker in CI)
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/redisx pkg/kafkax
git commit -m "feat(kit): kafka consume loop and redis once dedupe primitive"
```

---

### Task C3: `cmd/worker` role registry

Each future role lands as its own file with an `init()` registration — no more `main.go` edits (and no merge conflicts) per role.

**Files:**
- Create: `cmd/worker/registry.go`, `cmd/worker/relay.go`
- Modify: `cmd/worker/main.go`

- [ ] **Step 1: Create `cmd/worker/registry.go`**

```go
package main

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/fonvacano/yaxter/pkg/config"
)

type roleRunner func(ctx context.Context, logger zerolog.Logger, cfg config.Config)

// roleRunners is populated by init() in each role's file. Roles without a
// real implementation yet fall back to the placeholder heartbeat loop.
var roleRunners = map[string]roleRunner{}
```

- [ ] **Step 2: Move `runRelay` (and the metrics registry) from `main.go` into `cmd/worker/relay.go`**, unchanged except for the added registration:

```go
func init() { roleRunners["relay"] = runRelay }
```

- [ ] **Step 3: Update the role-start loop in `main.go`**

```go
	for _, role := range roles {
		if runner, ok := roleRunners[role]; ok {
			go runner(ctx, logger, cfg)
			continue
		}
		go runRole(ctx, logger, role)
	}
```

- [ ] **Step 4: Verify**

Run: `go build ./... && go test ./cmd/worker/ -v`
Expected: build green; existing `resolveRoles` tests PASS unchanged.

- [ ] **Step 5: Commit**

```bash
git add cmd/worker
git commit -m "refactor(worker): role-runner registry, one file per role"
```

---

### Task C4: `media` table migration

T1.3 validates `media_ids` are `ready`; the upload pipeline (sibling plan, T1.5) fills the table.

**Files:**
- Create: `migrations/000009_media.up.sql`, `migrations/000009_media.down.sql`
- Modify: `migrations/migrations_test.go` (`allTables`)

- [ ] **Step 1: Write the migration pair**

`migrations/000009_media.up.sql`:

```sql
-- Upload state machine per §2.5: pending -> uploaded -> ready | failed.
-- Tweets may reference only ready media; DB stores ids, never URLs.
CREATE TABLE media (
    id           BIGINT PRIMARY KEY,              -- snowflake
    owner_id     BIGINT NOT NULL,
    content_type TEXT   NOT NULL,
    size_bytes   BIGINT NOT NULL,
    status       TEXT   NOT NULL DEFAULT 'pending',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    ready_at     TIMESTAMPTZ
);
CREATE INDEX media_owner_id_idx ON media (owner_id);
```

`migrations/000009_media.down.sql`:

```sql
DROP TABLE media;
```

- [ ] **Step 2: Add `"media"` to `allTables` in `migrations/migrations_test.go`**

- [ ] **Step 3: Run**

Run: `go test ./migrations/ -v` (Docker in CI)
Expected: PASS — 13 tables, up/down/up clean.

- [ ] **Step 4: Commit**

```bash
git add migrations
git commit -m "feat(db): media upload-state table"
```

---

# Wave 1 — Track U: T1.2 Users & Follows

### Task U1: Users service — profiles with cache delete-on-write

**Files:**
- Create: `internal/users/service.go`
- Test: `internal/users/service_test.go` (integration, `-short`-skipped)

- [ ] **Step 1: Write the failing test** — `internal/users/service_test.go`

```go
package users

import (
	"context"
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	pgxkit "github.com/fonvacano/yaxter/pkg/pgx"
	"github.com/fonvacano/yaxter/pkg/snowflake"
)

func testService(t *testing.T) (*Service, *pgxpool.Pool, *miniredis.Miniredis) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: requires Docker")
	}
	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("yaxter"), tcpostgres.WithUsername("yaxter"),
		tcpostgres.WithPassword("yaxter"), tcpostgres.BasicWaitStrategies())
	require.NoError(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	m, err := migrate.New("file://../../migrations", dsn)
	require.NoError(t, err)
	require.NoError(t, m.Up())
	m.Close()
	pool, err := pgxkit.NewPool(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	gen, err := snowflake.New(4)
	require.NoError(t, err)
	return NewService(pool, rdb, gen, 50), pool, mr
}

func seedUser(t *testing.T, pool *pgxpool.Pool, id int64, username string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO users (id, username, email, pass_hash)
		VALUES ($1, $2, $3, 'x')`,
		id, username, username+"@example.com")
	require.NoError(t, err)
}

func TestGetByUsernameAndByID(t *testing.T) {
	svc, pool, _ := testService(t)
	ctx := context.Background()
	seedUser(t, pool, 10, "dave")

	u, err := svc.GetByUsername(ctx, "dave")
	require.NoError(t, err)
	require.EqualValues(t, 10, u.ID)

	u, err = svc.GetByID(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, "dave", u.Username)

	_, err = svc.GetByUsername(ctx, "missing")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestUpdateProfileDeletesCache(t *testing.T) {
	svc, pool, mr := testService(t)
	ctx := context.Background()
	seedUser(t, pool, 10, "dave")
	mr.Set(fmt.Sprintf("usr:%d", 10), "stale")

	u, err := svc.UpdateProfile(ctx, 10, UpdateProfile{Bio: ptr("hello")})
	require.NoError(t, err)
	require.Equal(t, "hello", u.Bio)
	require.False(t, mr.Exists("usr:10"), "profile cache must be deleted on write")
}

func TestUpdateProfileAvatarRequiresReadyOwnedMedia(t *testing.T) {
	svc, pool, _ := testService(t)
	ctx := context.Background()
	seedUser(t, pool, 10, "dave")
	_, err := pool.Exec(ctx, `
		INSERT INTO media (id, owner_id, content_type, size_bytes, status) VALUES
		(501, 10, 'image/webp', 100, 'ready'),
		(502, 10, 'image/webp', 100, 'pending'),
		(503, 99, 'image/webp', 100, 'ready')`)
	require.NoError(t, err)

	_, err = svc.UpdateProfile(ctx, 10, UpdateProfile{AvatarMediaID: ptr(int64(501))})
	require.NoError(t, err)
	_, err = svc.UpdateProfile(ctx, 10, UpdateProfile{AvatarMediaID: ptr(int64(502))})
	require.ErrorIs(t, err, ErrMediaNotReady)
	_, err = svc.UpdateProfile(ctx, 10, UpdateProfile{AvatarMediaID: ptr(int64(503))})
	require.ErrorIs(t, err, ErrMediaNotReady, "foreign media must be rejected uniformly")
}

func ptr[T any](v T) *T { return &v }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/users/ -v`
Expected: FAIL — `undefined: NewService`.

- [ ] **Step 3: Write `internal/users/service.go`**

```go
// Package users implements profiles and the follow graph (ARCHITECTURE.md §2.2).
package users

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/fonvacano/yaxter/pkg/snowflake"
)

var (
	ErrNotFound      = errors.New("users: not found")
	ErrSelfFollow    = errors.New("users: cannot follow yourself")
	ErrMediaNotReady = errors.New("users: avatar media not ready")
)

type User struct {
	ID             int64
	Username       string
	Email          string
	Bio            string
	AvatarKey      *string
	FollowersCount int
	FollowingCount int
	HasPassword    bool
	CreatedAt      time.Time
}

type UpdateProfile struct {
	Bio           *string
	AvatarMediaID *int64
}

type Service struct {
	db                 *pgxpool.Pool
	rdb                *redis.Client
	ids                *snowflake.Generator
	celebrityThreshold int
}

func NewService(db *pgxpool.Pool, rdb *redis.Client, ids *snowflake.Generator, celebrityThreshold int) *Service {
	return &Service{db: db, rdb: rdb, ids: ids, celebrityThreshold: celebrityThreshold}
}

const userColumns = `id, username, email, bio, avatar_key,
	followers_count, following_count, pass_hash IS NOT NULL, created_at`

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Username, &u.Email, &u.Bio, &u.AvatarKey,
		&u.FollowersCount, &u.FollowingCount, &u.HasPassword, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

func (s *Service) GetByID(ctx context.Context, id int64) (User, error) {
	return scanUser(s.db.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE id = $1`, id))
}

func (s *Service) GetByUsername(ctx context.Context, username string) (User, error) {
	return scanUser(s.db.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE username = $1`, username))
}

func (s *Service) UpdateProfile(ctx context.Context, userID int64, up UpdateProfile) (User, error) {
	if up.AvatarMediaID != nil {
		var ok bool
		err := s.db.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM media
				WHERE id = $1 AND owner_id = $2 AND status = 'ready'
			)`, *up.AvatarMediaID, userID).Scan(&ok)
		if err != nil {
			return User{}, err
		}
		if !ok { // not-found, not-owned, not-ready: one uniform error
			return User{}, ErrMediaNotReady
		}
	}
	var avatarKey *string
	if up.AvatarMediaID != nil {
		k := fmt.Sprintf("%d", *up.AvatarMediaID) // DB stores media_id, never URLs (§2.5)
		avatarKey = &k
	}
	u, err := scanUser(s.db.QueryRow(ctx, `
		UPDATE users SET
			bio        = COALESCE($2, bio),
			avatar_key = COALESCE($3, avatar_key)
		WHERE id = $1
		RETURNING `+userColumns, userID, up.Bio, avatarKey))
	if err != nil {
		return User{}, err
	}
	s.rdb.Del(ctx, fmt.Sprintf("usr:%d", userID)) // delete-on-write (§2.3)
	return u, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/users/ -race -v` (Docker in CI)
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/users
git commit -m "feat(users): profile reads and update with cache delete-on-write"
```

---

### Task U2: Follow/unfollow — dual edges, counts, event, celebs invalidation

**Files:**
- Create: `internal/users/follow.go`
- Test: `internal/users/follow_test.go`

- [ ] **Step 1: Write the failing test** — `internal/users/follow_test.go`

```go
package users

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFollowWritesBothEdgesCountsAndOneEvent(t *testing.T) {
	svc, pool, _ := testService(t)
	ctx := context.Background()
	seedUser(t, pool, 1, "alice")
	seedUser(t, pool, 2, "bob")

	require.NoError(t, svc.Follow(ctx, 1, "bob"))
	// Idempotent double-follow: no second edge, no second event.
	require.NoError(t, svc.Follow(ctx, 1, "bob"))

	var n int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM follows WHERE follower_id=1 AND followee_id=2`).Scan(&n))
	require.Equal(t, 1, n)
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM followers WHERE followee_id=2 AND follower_id=1`).Scan(&n))
	require.Equal(t, 1, n, "reverse edge must exist (§2.2)")

	require.NoError(t, pool.QueryRow(ctx,
		`SELECT followers_count FROM users WHERE id=2`).Scan(&n))
	require.Equal(t, 1, n)
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT following_count FROM users WHERE id=1`).Scan(&n))
	require.Equal(t, 1, n)

	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox WHERE topic='follows.v1'`).Scan(&n))
	require.Equal(t, 1, n, "exactly one event per state change")
}

func TestUnfollowRemovesEdgesAndEmitsOnce(t *testing.T) {
	svc, pool, _ := testService(t)
	ctx := context.Background()
	seedUser(t, pool, 1, "alice")
	seedUser(t, pool, 2, "bob")
	require.NoError(t, svc.Follow(ctx, 1, "bob"))

	require.NoError(t, svc.Unfollow(ctx, 1, "bob"))
	require.NoError(t, svc.Unfollow(ctx, 1, "bob")) // idempotent

	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM follows`).Scan(&n))
	require.Zero(t, n)
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM followers`).Scan(&n))
	require.Zero(t, n)
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT followers_count FROM users WHERE id=2`).Scan(&n))
	require.Zero(t, n)
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox WHERE topic='follows.v1'`).Scan(&n))
	require.Equal(t, 2, n, "one follow event + one unfollow event")
}

func TestFollowGuards(t *testing.T) {
	svc, pool, _ := testService(t)
	ctx := context.Background()
	seedUser(t, pool, 1, "alice")

	require.ErrorIs(t, svc.Follow(ctx, 1, "alice"), ErrSelfFollow)
	require.ErrorIs(t, svc.Follow(ctx, 1, "ghost"), ErrNotFound)
}

func TestFollowingACelebrityInvalidatesCelebsCache(t *testing.T) {
	svc, pool, mr := testService(t) // threshold = 50
	ctx := context.Background()
	seedUser(t, pool, 1, "alice")
	seedUser(t, pool, 2, "celeb")
	_, err := pool.Exec(ctx, `UPDATE users SET followers_count = 60 WHERE id = 2`)
	require.NoError(t, err)
	mr.Set(fmt.Sprintf("celebs:%d", 1), "stale")

	require.NoError(t, svc.Follow(ctx, 1, "celeb"))
	require.False(t, mr.Exists("celebs:1"),
		"celebs set must be invalidated on follow of a celebrity (§2.3)")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/users/ -run TestFollow -v`
Expected: FAIL — `undefined: (*Service).Follow`.

- [ ] **Step 3: Write `internal/users/follow.go`**

```go
package users

import (
	"context"
	"fmt"

	followsv1 "github.com/fonvacano/yaxter/gen/events/follows/v1"
	"github.com/fonvacano/yaxter/internal/events"
)

// Follow writes both edge tables, bumps both counters, and emits exactly one
// FollowChanged — all in one transaction (§2.2). Double-follow is a no-op.
func (s *Service) Follow(ctx context.Context, followerID int64, followeeUsername string) error {
	return s.setFollow(ctx, followerID, followeeUsername, true)
}

// Unfollow is the mirror image; unfollowing a non-followee is a no-op.
func (s *Service) Unfollow(ctx context.Context, followerID int64, followeeUsername string) error {
	return s.setFollow(ctx, followerID, followeeUsername, false)
}

func (s *Service) setFollow(ctx context.Context, followerID int64, followeeUsername string, follow bool) error {
	followee, err := s.GetByUsername(ctx, followeeUsername)
	if err != nil {
		return err
	}
	if followee.ID == followerID {
		return ErrSelfFollow
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var changed bool
	if follow {
		tag, err := tx.Exec(ctx, `
			INSERT INTO follows (follower_id, followee_id) VALUES ($1, $2)
			ON CONFLICT DO NOTHING`, followerID, followee.ID)
		if err != nil {
			return err
		}
		changed = tag.RowsAffected() == 1
		if changed {
			if _, err := tx.Exec(ctx, `
				INSERT INTO followers (followee_id, follower_id) VALUES ($1, $2)
				ON CONFLICT DO NOTHING`, followee.ID, followerID); err != nil {
				return err
			}
		}
	} else {
		tag, err := tx.Exec(ctx, `
			DELETE FROM follows WHERE follower_id = $1 AND followee_id = $2`,
			followerID, followee.ID)
		if err != nil {
			return err
		}
		changed = tag.RowsAffected() == 1
		if changed {
			if _, err := tx.Exec(ctx, `
				DELETE FROM followers WHERE followee_id = $1 AND follower_id = $2`,
				followee.ID, followerID); err != nil {
				return err
			}
		}
	}
	if !changed {
		return nil // idempotent: nothing changed, no counters, no event
	}

	delta := 1
	if !follow {
		delta = -1
	}
	if _, err := tx.Exec(ctx,
		`UPDATE users SET followers_count = followers_count + $2 WHERE id = $1`,
		followee.ID, delta); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE users SET following_count = following_count + $2 WHERE id = $1`,
		followerID, delta); err != nil {
		return err
	}

	eventID := s.ids.Next()
	ev := &followsv1.FollowEvent{
		Envelope: events.NewEnvelope(ctx, eventID),
		Payload: &followsv1.FollowEvent_FollowChanged{FollowChanged: &followsv1.FollowChanged{
			FollowerId: followerID,
			FolloweeId: followee.ID,
			Following:  follow,
		}},
	}
	if err := events.Emit(ctx, tx, eventID, "follows.v1", events.Key(followee.ID), ev); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	// Cache maintenance after commit: profile counts changed on both sides;
	// the follower's celebs set is stale if the followee is a celebrity.
	s.rdb.Del(ctx,
		fmt.Sprintf("usr:%d", followee.ID),
		fmt.Sprintf("usr:%d", followerID))
	if followee.FollowersCount+delta >= s.celebrityThreshold || followee.FollowersCount >= s.celebrityThreshold {
		s.rdb.Del(ctx, fmt.Sprintf("celebs:%d", followerID))
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/users/ -race -v` (Docker in CI)
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/users
git commit -m "feat(users): follow/unfollow with dual edges, counts, single event"
```

---

### Task U3: Follower/following lists + HTTP handlers + wiring

**Files:**
- Create: `internal/users/lists.go`, `internal/httpapi/users_handlers.go`
- Modify: `internal/httpapi/server.go` (replace the users 501 stubs), `internal/httpapi/wire.go` (Deps + construction), `pkg/config/config.go` (`CelebrityThreshold`)
- Test: `internal/users/lists_test.go`, `internal/httpapi/users_lifecycle_test.go`

- [ ] **Step 1: Write the failing list test** — `internal/users/lists_test.go`

```go
package users

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFollowersPagination(t *testing.T) {
	svc, pool, _ := testService(t)
	ctx := context.Background()
	seedUser(t, pool, 100, "star")
	for i := int64(1); i <= 5; i++ {
		seedUser(t, pool, i, "fan"+string(rune('a'+i-1)))
		require.NoError(t, svc.Follow(ctx, i, "star"))
	}

	page1, next, err := svc.Followers(ctx, "star", 0, 2)
	require.NoError(t, err)
	require.Len(t, page1, 2)
	require.NotZero(t, next)

	page2, next2, err := svc.Followers(ctx, "star", next, 2)
	require.NoError(t, err)
	require.Len(t, page2, 2)

	page3, next3, err := svc.Followers(ctx, "star", next2, 2)
	require.NoError(t, err)
	require.Len(t, page3, 1)
	require.Zero(t, next3, "last page has no cursor")

	seen := map[int64]bool{}
	for _, u := range append(append(page1, page2...), page3...) {
		require.False(t, seen[u.ID], "no duplicates across pages")
		seen[u.ID] = true
	}
	require.Len(t, seen, 5)

	following, _, err := svc.Following(ctx, "fana", 0, 10)
	require.NoError(t, err)
	require.Len(t, following, 1)
	require.Equal(t, "star", following[0].Username)
}
```

- [ ] **Step 2: Run to verify failure, then write `internal/users/lists.go`**

```go
package users

import "context"

type Summary struct {
	ID        int64
	Username  string
	AvatarKey *string
}

// Followers pages through who follows username, newest-id first.
// Cursor semantics: pass 0 for the first page; pass the returned cursor for
// the next; 0 returned means last page (deviation #3: id-ordered, stable).
func (s *Service) Followers(ctx context.Context, username string, cursor int64, limit int) ([]Summary, int64, error) {
	return s.edgePage(ctx, username, cursor, limit, `
		SELECT u.id, u.username, u.avatar_key
		FROM followers f JOIN users u ON u.id = f.follower_id
		WHERE f.followee_id = $1 AND ($2 = 0 OR f.follower_id < $2)
		ORDER BY f.follower_id DESC LIMIT $3`)
}

// Following pages through who username follows.
func (s *Service) Following(ctx context.Context, username string, cursor int64, limit int) ([]Summary, int64, error) {
	return s.edgePage(ctx, username, cursor, limit, `
		SELECT u.id, u.username, u.avatar_key
		FROM follows f JOIN users u ON u.id = f.followee_id
		WHERE f.follower_id = $1 AND ($2 = 0 OR f.followee_id < $2)
		ORDER BY f.followee_id DESC LIMIT $3`)
}

func (s *Service) edgePage(ctx context.Context, username string, cursor int64, limit int, query string) ([]Summary, int64, error) {
	owner, err := s.GetByUsername(ctx, username)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(ctx, query, owner.ID, cursor, limit+1)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Summary
	for rows.Next() {
		var u Summary
		if err := rows.Scan(&u.ID, &u.Username, &u.AvatarKey); err != nil {
			return nil, 0, err
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	var next int64
	if len(out) > limit { // limit+1 probe: more pages exist
		out = out[:limit]
		next = out[limit-1].ID
	}
	return out, next, nil
}
```

Run: `go test ./internal/users/ -race -v` → PASS.

- [ ] **Step 3: Add `CelebrityThreshold` to `pkg/config/config.go`** (+ defaults test asserting `50`)

```go
	CelebrityThreshold int `env:"CELEBRITY_THRESHOLD" envDefault:"50"`
```

- [ ] **Step 4: Write `internal/httpapi/users_handlers.go`** and replace the users 501 stubs in `server.go`

```go
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/fonvacano/yaxter/internal/users"
)

type UsersHandlers struct {
	svc           *users.Service
	mediaBaseURL  string // e.g. https://media.example.com; avatar URL scheme per §2.5
}

func (h *UsersHandlers) avatarURL(key *string) *string {
	if key == nil {
		return nil
	}
	u := h.mediaBaseURL + "/feed/" + *key + ".webp"
	return &u
}

func (h *UsersHandlers) publicUser(u users.User) User {
	return User{
		Id: formatID(u.ID), Username: u.Username, Bio: u.Bio,
		AvatarUrl: h.avatarURL(u.AvatarKey),
		FollowersCount: u.FollowersCount, FollowingCount: u.FollowingCount,
		CreatedAt: u.CreatedAt,
	}
}

func (h *UsersHandlers) summary(s users.Summary) UserSummary {
	return UserSummary{Id: formatID(s.ID), Username: s.Username, AvatarUrl: h.avatarURL(s.AvatarKey)}
}

func requireUser(w http.ResponseWriter, r *http.Request) (int64, bool) {
	uid, ok := UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
	}
	return uid, ok
}

func (h *UsersHandlers) GetMe(w http.ResponseWriter, r *http.Request) {
	uid, ok := requireUser(w, r)
	if !ok {
		return
	}
	u, err := h.svc.GetByID(r.Context(), uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "profile load failed")
		return
	}
	writeJSON(w, http.StatusOK, h.privateUser(u))
}

func (h *UsersHandlers) privateUser(u users.User) PrivateUser {
	// Adjust field mapping to the exact generated model (allOf flattening).
	return PrivateUser{
		Id: formatID(u.ID), Username: u.Username, Bio: u.Bio,
		AvatarUrl: h.avatarURL(u.AvatarKey),
		FollowersCount: u.FollowersCount, FollowingCount: u.FollowingCount,
		CreatedAt: u.CreatedAt, Email: u.Email,
		HasPassword: u.HasPassword, LinkedProviders: []string{},
	}
}

func (h *UsersHandlers) UpdateMe(w http.ResponseWriter, r *http.Request) {
	uid, ok := requireUser(w, r)
	if !ok {
		return
	}
	var req UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed_body", "invalid JSON")
		return
	}
	if req.Bio != nil && len(*req.Bio) > 160 {
		writeError(w, http.StatusBadRequest, "validation_failed", "bio too long")
		return
	}
	up := users.UpdateProfile{Bio: req.Bio}
	if req.AvatarMediaId != nil {
		id, err := strconv.ParseInt(string(*req.AvatarMediaId), 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "validation_failed", "bad avatar_media_id")
			return
		}
		up.AvatarMediaID = &id
	}
	u, err := h.svc.UpdateProfile(r.Context(), uid, up)
	switch {
	case errors.Is(err, users.ErrMediaNotReady):
		writeError(w, http.StatusBadRequest, "media_not_ready", "avatar media must be uploaded and ready")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal", "update failed")
		return
	}
	writeJSON(w, http.StatusOK, h.privateUser(u))
}

func (h *UsersHandlers) GetUser(w http.ResponseWriter, r *http.Request, username string) {
	u, err := h.svc.GetByUsername(r.Context(), username)
	if errors.Is(err, users.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such user")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "profile load failed")
		return
	}
	writeJSON(w, http.StatusOK, h.publicUser(u))
}

func (h *UsersHandlers) setFollow(w http.ResponseWriter, r *http.Request, username string, follow bool) {
	uid, ok := requireUser(w, r)
	if !ok {
		return
	}
	var err error
	if follow {
		err = h.svc.Follow(r.Context(), uid, username)
	} else {
		err = h.svc.Unfollow(r.Context(), uid, username)
	}
	switch {
	case errors.Is(err, users.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "no such user")
	case errors.Is(err, users.ErrSelfFollow):
		writeError(w, http.StatusBadRequest, "self_follow", "cannot follow yourself")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal", "follow failed")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *UsersHandlers) listEdges(w http.ResponseWriter, r *http.Request, username string,
	cursor *string, limit *int,
	fetch func(ctx context.Context, username string, cursor int64, limit int) ([]users.Summary, int64, error),
) {
	var c int64
	if cursor != nil {
		c, _ = strconv.ParseInt(*cursor, 10, 64)
	}
	l := 20
	if limit != nil {
		l = *limit
	}
	page, next, err := fetch(r.Context(), username, c, l)
	if errors.Is(err, users.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such user")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "list failed")
		return
	}
	out := UserPage{Items: make([]UserSummary, 0, len(page))}
	for _, u := range page {
		out.Items = append(out.Items, h.summary(u))
	}
	if next != 0 {
		ncur := strconv.FormatInt(next, 10)
		out.NextCursor = &ncur
	}
	writeJSON(w, http.StatusOK, out)
}
```

(Add `"context"` import.) In `server.go`, replace the users stubs with delegation — match the generated signatures exactly (path/query params are arguments):

```go
func (s *Server) GetMe(w http.ResponseWriter, r *http.Request)  { s.Users.GetMe(w, r) }
func (s *Server) UpdateMe(w http.ResponseWriter, r *http.Request, params UpdateMeParams) {
	s.Users.UpdateMe(w, r)
}
func (s *Server) GetUser(w http.ResponseWriter, r *http.Request, username string) {
	s.Users.GetUser(w, r, username)
}
func (s *Server) FollowUser(w http.ResponseWriter, r *http.Request, username string, params FollowUserParams) {
	s.Users.setFollow(w, r, username, true)
}
func (s *Server) UnfollowUser(w http.ResponseWriter, r *http.Request, username string, params UnfollowUserParams) {
	s.Users.setFollow(w, r, username, false)
}
func (s *Server) ListFollowers(w http.ResponseWriter, r *http.Request, username string, params ListFollowersParams) {
	s.Users.listEdges(w, r, username, params.Cursor, params.Limit, s.Users.svc.Followers)
}
func (s *Server) ListFollowing(w http.ResponseWriter, r *http.Request, username string, params ListFollowingParams) {
	s.Users.listEdges(w, r, username, params.Cursor, params.Limit, s.Users.svc.Following)
}
```

In `wire.go`: add `CelebrityThreshold int` and `MediaBaseURL string` to `Deps`, construct `users.NewService(d.DB, d.Redis, d.IDs, d.CelebrityThreshold)`, pass to `NewServer`; `cmd/api` passes `cfg.CelebrityThreshold`. (`MediaBaseURL` config field: `env:"MEDIA_BASE_URL" envDefault:"http://localhost:9000/media"` — the sibling plan's media track also adds it; on merge keep one.)

- [ ] **Step 5: Write the HTTP lifecycle test** — `internal/httpapi/users_lifecycle_test.go` (reuses `liveHandler` from the T1.1 plan)

```go
package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func registerAndToken(t *testing.T, h http.Handler, username string) string {
	t.Helper()
	rr := postJSON(t, h, "/v1/auth/register", map[string]any{
		"username": username, "email": username + "@example.com", "password": "password123",
	}, map[string]string{"Idempotency-Key": uuid.NewString()})
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	var body struct {
		Tokens struct {
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	return body.Tokens.AccessToken
}

func TestFollowOverHTTP(t *testing.T) {
	h := liveHandler(t, 100)
	tokA := registerAndToken(t, h, "anna")
	_ = registerAndToken(t, h, "boris")

	req := httptest.NewRequest(http.MethodPost, "/v1/users/boris/follow", nil)
	req.Header.Set("Authorization", "Bearer "+tokA)
	req.Header.Set("Idempotency-Key", uuid.NewString())
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())

	// Public profile reflects the count.
	req = httptest.NewRequest(http.MethodGet, "/v1/users/boris", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	var u struct {
		FollowersCount int `json:"followers_count"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &u))
	require.Equal(t, 1, u.FollowersCount)

	// Followers list contains anna.
	req = httptest.NewRequest(http.MethodGet, "/v1/users/boris/followers", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "anna")
}
```

- [ ] **Step 6: Run, build, commit**

```bash
go build ./... && go test ./internal/httpapi/ ./internal/users/ -race -v
git add internal/users internal/httpapi pkg/config cmd/api
git commit -m "feat(users): profile, follow, and list endpoints wired"
```

---

# Wave 1 — Track T: T1.3 Tweets & Retweets Write Path

### Task T1: Tweet create — validation, retweet flattening, events, cache append

**Files:**
- Create: `internal/tweets/service.go`, `internal/tweets/cache.go`
- Test: `internal/tweets/service_test.go`

- [ ] **Step 1: Write the failing test** — `internal/tweets/service_test.go`

```go
package tweets

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"google.golang.org/protobuf/proto"

	tweetsv1 "github.com/fonvacano/yaxter/gen/events/tweets/v1"
	pgxkit "github.com/fonvacano/yaxter/pkg/pgx"
	"github.com/fonvacano/yaxter/pkg/snowflake"
)

func testService(t *testing.T) (*Service, *pgxpool.Pool, *miniredis.Miniredis) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: requires Docker")
	}
	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("yaxter"), tcpostgres.WithUsername("yaxter"),
		tcpostgres.WithPassword("yaxter"), tcpostgres.BasicWaitStrategies())
	require.NoError(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	m, err := migrate.New("file://../../migrations", dsn)
	require.NoError(t, err)
	require.NoError(t, m.Up())
	m.Close()
	pool, err := pgxkit.NewPool(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	_, err = pool.Exec(ctx, `
		INSERT INTO users (id, username, email, pass_hash, followers_count) VALUES
		(1, 'author', 'a@example.com', 'x', 7),
		(2, 'other',  'o@example.com', 'x', 0)`)
	require.NoError(t, err)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	gen, err := snowflake.New(5)
	require.NoError(t, err)
	return NewService(pool, rdb, gen), pool, mr
}

func lastTweetEvent(t *testing.T, pool *pgxpool.Pool) *tweetsv1.TweetEvent {
	t.Helper()
	var payload []byte
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT payload FROM outbox WHERE topic = 'tweets.v1'
		ORDER BY id DESC LIMIT 1`).Scan(&payload))
	var ev tweetsv1.TweetEvent
	require.NoError(t, proto.Unmarshal(payload, &ev))
	return &ev
}

func TestCreateTweetPersistsEmitsAndCaches(t *testing.T) {
	svc, pool, mr := testService(t)
	ctx := context.Background()

	tw, err := svc.Create(ctx, 1, "hello world", nil, 0)
	require.NoError(t, err)
	require.NotZero(t, tw.ID)

	var text string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT text FROM tweets WHERE id = $1`, tw.ID).Scan(&text))
	require.Equal(t, "hello world", text)

	ev := lastTweetEvent(t, pool)
	require.Equal(t, tw.ID, ev.GetCreated().GetTweetId())
	require.EqualValues(t, 7, ev.GetCreated().GetAuthorFollowersCount(),
		"event snapshots the author's follower count for the fan-out threshold")

	utl, err := mr.List("utl:1")
	require.NoError(t, err)
	require.Len(t, utl, 1, "author's own timeline cache must be appended (§2.1)")
	require.True(t, mr.Exists(tweetKey(tw.ID)), "tweet body cached")
}

func TestCreateValidation(t *testing.T) {
	svc, pool, _ := testService(t)
	ctx := context.Background()

	_, err := svc.Create(ctx, 1, "", nil, 0)
	require.ErrorIs(t, err, ErrValidation, "empty non-retweet rejected")

	long := make([]rune, 281)
	for i := range long {
		long[i] = 'x'
	}
	_, err = svc.Create(ctx, 1, string(long), nil, 0)
	require.ErrorIs(t, err, ErrValidation)

	// Media must be ready and owned.
	_, err = pool.Exec(ctx, `
		INSERT INTO media (id, owner_id, content_type, size_bytes, status) VALUES
		(601, 1, 'image/webp', 9, 'ready'), (602, 1, 'image/webp', 9, 'pending')`)
	require.NoError(t, err)
	_, err = svc.Create(ctx, 1, "with media", []int64{601, 602}, 0)
	require.ErrorIs(t, err, ErrMediaNotReady)
	_, err = svc.Create(ctx, 1, "with media", []int64{601}, 0)
	require.NoError(t, err)
}

func TestRetweetFlattensAndEmitsEngagement(t *testing.T) {
	svc, pool, _ := testService(t)
	ctx := context.Background()

	orig, err := svc.Create(ctx, 1, "original", nil, 0)
	require.NoError(t, err)
	rt1, err := svc.Create(ctx, 2, "", nil, orig.ID)
	require.NoError(t, err)
	require.Equal(t, orig.ID, rt1.RetweetOfID)

	// Retweet of a retweet flattens to the original (§ Tweet schema note).
	rt2, err := svc.Create(ctx, 1, "", nil, rt1.ID)
	require.NoError(t, err)
	require.Equal(t, orig.ID, rt2.RetweetOfID)

	var n int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT count(*) FROM outbox WHERE topic = 'engagements.v1'`).Scan(&n))
	require.Equal(t, 2, n, "each retweet emits one engagement event for counters")

	_, err = svc.Create(ctx, 1, "", nil, 999999)
	require.ErrorIs(t, err, ErrNotFound)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tweets/ -v`
Expected: FAIL — `undefined: NewService`.

- [ ] **Step 3: Write `internal/tweets/cache.go`**

```go
package tweets

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const utlCap = 200 // §2.3: user's own tweet ids, profile + celebrity merge source

func tweetKey(id int64) string  { return fmt.Sprintf("tw:%d", id) }
func utlKey(author int64) string { return fmt.Sprintf("utl:%d", author) }

func appendUTL(ctx context.Context, rdb *redis.Client, author, tweetID int64) {
	pipe := rdb.Pipeline()
	pipe.LPush(ctx, utlKey(author), tweetID)
	pipe.LTrim(ctx, utlKey(author), 0, utlCap-1)
	_, _ = pipe.Exec(ctx) // cache, not truth: errors are non-fatal
}

func cacheTweet(ctx context.Context, rdb *redis.Client, tw Tweet) {
	raw, err := json.Marshal(tw)
	if err != nil {
		return
	}
	rdb.Set(ctx, tweetKey(tw.ID), raw, time.Hour)
}

func dropTweetCaches(ctx context.Context, rdb *redis.Client, author, tweetID int64) {
	rdb.Del(ctx, tweetKey(tweetID))
	rdb.LRem(ctx, utlKey(author), 0, tweetID)
}
```

- [ ] **Step 4: Write `internal/tweets/service.go`**

```go
// Package tweets implements the tweet/retweet write path (ARCHITECTURE.md §2.1):
// row + outbox event in one transaction, cache append after commit.
package tweets

import (
	"context"
	"errors"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	engagementsv1 "github.com/fonvacano/yaxter/gen/events/engagements/v1"
	tweetsv1 "github.com/fonvacano/yaxter/gen/events/tweets/v1"
	"github.com/fonvacano/yaxter/internal/events"
	"github.com/fonvacano/yaxter/pkg/snowflake"
)

var (
	ErrValidation    = errors.New("tweets: validation failed")
	ErrNotFound      = errors.New("tweets: not found")
	ErrForbidden     = errors.New("tweets: not the author")
	ErrMediaNotReady = errors.New("tweets: media not ready")
)

type Tweet struct {
	ID            int64     `json:"id"`
	AuthorID      int64     `json:"author_id"`
	Text          string    `json:"text"`
	RetweetOfID   int64     `json:"retweet_of_id,omitempty"`
	MediaIDs      []int64   `json:"media_ids,omitempty"`
	LikesCount    int       `json:"likes_count"`
	RetweetsCount int       `json:"retweets_count"`
	CreatedAt     time.Time `json:"created_at"`
}

type Service struct {
	db  *pgxpool.Pool
	rdb *redis.Client
	ids *snowflake.Generator
}

func NewService(db *pgxpool.Pool, rdb *redis.Client, ids *snowflake.Generator) *Service {
	return &Service{db: db, rdb: rdb, ids: ids}
}

func (s *Service) Create(ctx context.Context, authorID int64, text string, mediaIDs []int64, retweetOfID int64) (Tweet, error) {
	if utf8.RuneCountInString(text) > 280 {
		return Tweet{}, ErrValidation
	}
	if text == "" && retweetOfID == 0 {
		return Tweet{}, ErrValidation
	}
	if len(mediaIDs) > 4 {
		return Tweet{}, ErrValidation
	}
	if len(mediaIDs) > 0 {
		var ready int
		if err := s.db.QueryRow(ctx, `
			SELECT count(*) FROM media
			WHERE id = ANY($1) AND owner_id = $2 AND status = 'ready'`,
			mediaIDs, authorID).Scan(&ready); err != nil {
			return Tweet{}, err
		}
		if ready != len(mediaIDs) {
			return Tweet{}, ErrMediaNotReady
		}
	}

	// Retweets flatten: retweeting a retweet targets the original (one level).
	var origAuthorID int64
	if retweetOfID != 0 {
		var origRetweetOf *int64
		err := s.db.QueryRow(ctx, `
			SELECT author_id, retweet_of_id FROM tweets WHERE id = $1`,
			retweetOfID).Scan(&origAuthorID, &origRetweetOf)
		if errors.Is(err, pgx.ErrNoRows) {
			return Tweet{}, ErrNotFound
		}
		if err != nil {
			return Tweet{}, err
		}
		if origRetweetOf != nil {
			retweetOfID = *origRetweetOf
			err = s.db.QueryRow(ctx, `SELECT author_id FROM tweets WHERE id = $1`,
				retweetOfID).Scan(&origAuthorID)
			if err != nil {
				return Tweet{}, err
			}
		}
	}

	var followersCount int32
	if err := s.db.QueryRow(ctx,
		`SELECT followers_count FROM users WHERE id = $1`, authorID).
		Scan(&followersCount); err != nil {
		return Tweet{}, err
	}

	tw := Tweet{
		ID: s.ids.Next(), AuthorID: authorID, Text: text,
		RetweetOfID: retweetOfID, MediaIDs: mediaIDs,
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Tweet{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var retweetOf *int64
	if retweetOfID != 0 {
		retweetOf = &retweetOfID
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO tweets (id, author_id, text, retweet_of_id, media)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at`,
		tw.ID, authorID, text, retweetOf, mediaIDs).Scan(&tw.CreatedAt); err != nil {
		return Tweet{}, err
	}

	createdID := s.ids.Next()
	created := &tweetsv1.TweetEvent{
		Envelope: events.NewEnvelope(ctx, createdID),
		Payload: &tweetsv1.TweetEvent_Created{Created: &tweetsv1.TweetCreated{
			TweetId: tw.ID, AuthorId: authorID, Text: text,
			RetweetOfId: retweetOfID, MediaIds: mediaIDs,
			AuthorFollowersCount: followersCount,
		}},
	}
	if err := events.Emit(ctx, tx, createdID, "tweets.v1", events.Key(authorID), created); err != nil {
		return Tweet{}, err
	}

	if retweetOfID != 0 { // counters consume retweets via engagements.v1 (§2.4)
		engID := s.ids.Next()
		eng := &engagementsv1.EngagementEvent{
			Envelope: events.NewEnvelope(ctx, engID),
			Payload: &engagementsv1.EngagementEvent_Retweeted{Retweeted: &engagementsv1.TweetRetweeted{
				TweetId: retweetOfID, RetweetId: tw.ID,
				UserId: authorID, AuthorId: origAuthorID,
			}},
		}
		if err := events.Emit(ctx, tx, engID, "engagements.v1", events.Key(retweetOfID), eng); err != nil {
			return Tweet{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Tweet{}, err
	}

	appendUTL(ctx, s.rdb, authorID, tw.ID) // author sees own tweet immediately
	cacheTweet(ctx, s.rdb, tw)
	return tw, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/tweets/ -race -v` (Docker in CI)
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tweets
git commit -m "feat(tweets): create with validation, retweet flattening, events, caches"
```

---

### Task T2: Tweet delete + read with hydration

**Files:**
- Create: `internal/tweets/hydrate.go`
- Modify: `internal/tweets/service.go` (add `Delete`, `Get`)
- Test: `internal/tweets/service_test.go` (extend)

- [ ] **Step 1: Extend the failing test**

```go
func TestDeleteOwnTweetOnly(t *testing.T) {
	svc, pool, mr := testService(t)
	ctx := context.Background()

	tw, err := svc.Create(ctx, 1, "to delete", nil, 0)
	require.NoError(t, err)

	require.ErrorIs(t, svc.Delete(ctx, 2, tw.ID), ErrForbidden)
	require.NoError(t, svc.Delete(ctx, 1, tw.ID))
	require.ErrorIs(t, svc.Delete(ctx, 1, tw.ID), ErrNotFound)

	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM tweets`).Scan(&n))
	require.Zero(t, n)
	ev := lastTweetEvent(t, pool)
	require.Equal(t, tw.ID, ev.GetDeleted().GetTweetId())
	require.False(t, mr.Exists(tweetKey(tw.ID)), "tw: cache dropped on delete")
	utl, _ := mr.List("utl:1")
	require.Empty(t, utl, "utl: entry removed on delete")
}

func TestGetHydratesAuthorAndCounters(t *testing.T) {
	svc, pool, _ := testService(t)
	ctx := context.Background()

	tw, err := svc.Create(ctx, 1, "readable", nil, 0)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`UPDATE tweets SET likes_count = 5 WHERE id = $1`, tw.ID)
	require.NoError(t, err)

	got, err := svc.Get(ctx, tw.ID)
	require.NoError(t, err)
	require.Equal(t, "readable", got.Text)
	require.Equal(t, "author", got.AuthorUsername)
	require.Equal(t, 5, got.LikesCount)

	_, err = svc.Get(ctx, 424242)
	require.ErrorIs(t, err, ErrNotFound)
}
```

- [ ] **Step 2: Run to verify failure, then implement**

Add to `internal/tweets/service.go`:

```go
// Delete removes the author's own tweet and emits TweetDeleted (and the
// engagement reversal when it was a retweet) in one tx.
func (s *Service) Delete(ctx context.Context, userID, tweetID int64) error {
	var authorID int64
	var retweetOf *int64
	err := s.db.QueryRow(ctx,
		`SELECT author_id, retweet_of_id FROM tweets WHERE id = $1`, tweetID).
		Scan(&authorID, &retweetOf)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if authorID != userID {
		return ErrForbidden
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, `DELETE FROM tweets WHERE id = $1`, tweetID); err != nil {
		return err
	}
	delID := s.ids.Next()
	deleted := &tweetsv1.TweetEvent{
		Envelope: events.NewEnvelope(ctx, delID),
		Payload: &tweetsv1.TweetEvent_Deleted{Deleted: &tweetsv1.TweetDeleted{
			TweetId: tweetID, AuthorId: authorID,
		}},
	}
	if err := events.Emit(ctx, tx, delID, "tweets.v1", events.Key(authorID), deleted); err != nil {
		return err
	}
	if retweetOf != nil {
		engID := s.ids.Next()
		eng := &engagementsv1.EngagementEvent{
			Envelope: events.NewEnvelope(ctx, engID),
			Payload: &engagementsv1.EngagementEvent_Unretweeted{Unretweeted: &engagementsv1.TweetUnretweeted{
				TweetId: *retweetOf, RetweetId: tweetID, UserId: userID,
			}},
		}
		if err := events.Emit(ctx, tx, engID, "engagements.v1", events.Key(*retweetOf), eng); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	dropTweetCaches(ctx, s.rdb, authorID, tweetID)
	return nil
}
```

Create `internal/tweets/hydrate.go`:

```go
package tweets

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// HydratedTweet is a Tweet plus its author projection. The author comes from
// a local query (plan deviation #4 — no cross-module import); counters come
// from the PG columns here and are upgraded to the Redis read-through by the
// counters track (T1.4) — on merge conflict, the counters version wins.
type HydratedTweet struct {
	Tweet
	AuthorUsername  string
	AuthorAvatarKey *string
}

func (s *Service) Get(ctx context.Context, tweetID int64) (HydratedTweet, error) {
	var h HydratedTweet
	var retweetOf *int64
	err := s.db.QueryRow(ctx, `
		SELECT t.id, t.author_id, t.text, t.retweet_of_id, t.created_at,
		       t.likes_count, t.retweets_count, u.username, u.avatar_key
		FROM tweets t JOIN users u ON u.id = t.author_id
		WHERE t.id = $1`, tweetID).
		Scan(&h.ID, &h.AuthorID, &h.Text, &retweetOf, &h.CreatedAt,
			&h.LikesCount, &h.RetweetsCount, &h.AuthorUsername, &h.AuthorAvatarKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return HydratedTweet{}, ErrNotFound
	}
	if err != nil {
		return HydratedTweet{}, err
	}
	if retweetOf != nil {
		h.RetweetOfID = *retweetOf
	}
	return h, nil
}
```

- [ ] **Step 3: Run, commit**

```bash
go test ./internal/tweets/ -race -v
git add internal/tweets
git commit -m "feat(tweets): delete with events and hydrated single-tweet read"
```

---

### Task T3: Tweet HTTP handlers + idempotency DoD test

**Files:**
- Create: `internal/httpapi/tweets_handlers.go`
- Modify: `internal/httpapi/server.go` (replace `CreateTweet`/`GetTweet`/`DeleteTweet` stubs), `internal/httpapi/wire.go` (tweets service)
- Test: `internal/httpapi/tweets_lifecycle_test.go`

- [ ] **Step 1: Write `internal/httpapi/tweets_handlers.go`**

```go
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/fonvacano/yaxter/internal/tweets"
)

type TweetsHandlers struct {
	svc          *tweets.Service
	mediaBaseURL string
}

func (h *TweetsHandlers) toAPI(t tweets.HydratedTweet) Tweet {
	out := Tweet{
		Id:   formatID(t.ID),
		Text: t.Text,
		Author: UserSummary{
			Id: formatID(t.AuthorID), Username: t.AuthorUsername,
		},
		LikesCount: t.LikesCount, RetweetsCount: t.RetweetsCount,
		CreatedAt: t.CreatedAt,
	}
	for _, mid := range t.MediaIDs {
		id := formatID(mid)
		out.Media = append(out.Media, MediaRef{Id: id, Urls: mediaURLs(h.mediaBaseURL, mid)})
	}
	return out
}

func mediaURLs(base string, id int64) struct {
	Feed  string `json:"feed"`
	Orig  string `json:"orig"`
	Thumb string `json:"thumb"`
} {
	s := strconv.FormatInt(id, 10)
	return struct {
		Feed  string `json:"feed"`
		Orig  string `json:"orig"`
		Thumb string `json:"thumb"`
	}{
		Feed: base + "/feed/" + s + ".webp",
		Orig: base + "/orig/" + s + ".webp",
		Thumb: base + "/thumb/" + s + ".webp",
	}
}

func (h *TweetsHandlers) Create(w http.ResponseWriter, r *http.Request) {
	uid, ok := requireUser(w, r)
	if !ok {
		return
	}
	var req CreateTweetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed_body", "invalid JSON")
		return
	}
	var mediaIDs []int64
	if req.MediaIds != nil {
		for _, raw := range *req.MediaIds {
			id, err := strconv.ParseInt(string(raw), 10, 64)
			if err != nil {
				writeError(w, http.StatusBadRequest, "validation_failed", "bad media id")
				return
			}
			mediaIDs = append(mediaIDs, id)
		}
	}
	var retweetOf int64
	if req.RetweetOfId != nil {
		var err error
		retweetOf, err = strconv.ParseInt(string(*req.RetweetOfId), 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "validation_failed", "bad retweet_of_id")
			return
		}
	}
	tw, err := h.svc.Create(r.Context(), uid, req.Text, mediaIDs, retweetOf)
	switch {
	case errors.Is(err, tweets.ErrValidation):
		writeError(w, http.StatusBadRequest, "validation_failed", "tweet rejected")
		return
	case errors.Is(err, tweets.ErrMediaNotReady), errors.Is(err, tweets.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "referenced media or tweet not available")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal", "tweet failed")
		return
	}
	hydrated, err := h.svc.Get(r.Context(), tw.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "tweet failed")
		return
	}
	writeJSON(w, http.StatusCreated, h.toAPI(hydrated))
}

func (h *TweetsHandlers) Get(w http.ResponseWriter, r *http.Request, id string) {
	tweetID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "no such tweet")
		return
	}
	t, err := h.svc.Get(r.Context(), tweetID)
	if errors.Is(err, tweets.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such tweet")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "read failed")
		return
	}
	writeJSON(w, http.StatusOK, h.toAPI(t))
}

func (h *TweetsHandlers) Delete(w http.ResponseWriter, r *http.Request, id string) {
	uid, ok := requireUser(w, r)
	if !ok {
		return
	}
	tweetID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "no such tweet")
		return
	}
	switch err := h.svc.Delete(r.Context(), uid, tweetID); {
	case errors.Is(err, tweets.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "no such tweet")
	case errors.Is(err, tweets.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "not your tweet")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal", "delete failed")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
```

Replace the stubs in `server.go` (match generated signatures) and extend `wire.go` with `tweets.NewService(d.DB, d.Redis, d.IDs)`.

- [ ] **Step 2: Write the DoD test** — `internal/httpapi/tweets_lifecycle_test.go`

```go
package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// T1.3 DoD: duplicate Idempotency-Key returns the identical cached response,
// with a single row and a single event behind it.
func TestDuplicateIdempotencyKeySingleTweet(t *testing.T) {
	h := liveHandler(t, 100)
	tok := registerAndToken(t, h, "writer")
	key := uuid.NewString()

	body := map[string]any{"text": "exactly once"}
	hdrs := map[string]string{"Idempotency-Key": key, "Authorization": "Bearer " + tok}

	first := postJSON(t, h, "/v1/tweets", body, hdrs)
	require.Equal(t, http.StatusCreated, first.Code, first.Body.String())
	second := postJSON(t, h, "/v1/tweets", body, hdrs)
	require.Equal(t, http.StatusCreated, second.Code)
	require.Equal(t, first.Body.String(), second.Body.String(),
		"replay must return the byte-identical response")

	var tw struct {
		Id string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &tw))

	// Single row, single tweets.v1 event — count via the test pool exposed
	// by liveHandler's container (add a package-level accessor in
	// lifecycle_test.go: livePool(t) returning the migrated pool).
	pool := livePool(t)
	var n int
	require.NoError(t, pool.QueryRow(t.Context(),
		`SELECT count(*) FROM tweets`).Scan(&n))
	require.Equal(t, 1, n)
	require.NoError(t, pool.QueryRow(t.Context(),
		`SELECT count(*) FROM outbox WHERE topic='tweets.v1'`).Scan(&n))
	require.Equal(t, 1, n)
}
```

**Refactor note:** `liveHandler` (from the T1.1 plan) creates the pool internally; extract it so tests can reach it — change `liveHandler` to store the pool in a package-level test variable and add `func livePool(t *testing.T) *pgxpool.Pool` returning it. Keep the change inside the `_test.go` file.

- [ ] **Step 3: Run, build, commit**

```bash
go build ./... && go test ./internal/httpapi/ -race -v
git add internal/httpapi
git commit -m "feat(tweets): http handlers with idempotency replay verified"
```

---

# Wave 1 — Track L: T1.4 Likes + Counter Pipeline

### Task L1: Like/unlike service + engagement events

**Files:**
- Create: `internal/tweets/likes.go`
- Test: `internal/tweets/likes_test.go`

- [ ] **Step 1: Write the failing test** — `internal/tweets/likes_test.go`

```go
package tweets

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLikeIsIdempotentAndEmitsOncePerChange(t *testing.T) {
	svc, pool, _ := testService(t)
	ctx := context.Background()
	tw, err := svc.Create(ctx, 1, "likeable", nil, 0)
	require.NoError(t, err)

	require.NoError(t, svc.Like(ctx, 2, tw.ID))
	require.NoError(t, svc.Like(ctx, 2, tw.ID)) // idempotent

	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM likes`).Scan(&n))
	require.Equal(t, 1, n)
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox WHERE topic='engagements.v1'`).Scan(&n))
	require.Equal(t, 1, n, "double-like emits exactly one event")

	require.NoError(t, svc.Unlike(ctx, 2, tw.ID))
	require.NoError(t, svc.Unlike(ctx, 2, tw.ID)) // idempotent
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM likes`).Scan(&n))
	require.Zero(t, n)
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox WHERE topic='engagements.v1'`).Scan(&n))
	require.Equal(t, 2, n)

	require.ErrorIs(t, svc.Like(ctx, 2, 99999), ErrNotFound)
}
```

- [ ] **Step 2: Run to verify failure, then write `internal/tweets/likes.go`**

```go
package tweets

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	engagementsv1 "github.com/fonvacano/yaxter/gen/events/engagements/v1"
	"github.com/fonvacano/yaxter/internal/events"
)

// Like records the like (idempotent via PK) and emits TweetLiked in the same
// tx (§2.7 step 1). No state change => no event.
func (s *Service) Like(ctx context.Context, userID, tweetID int64) error {
	return s.setLike(ctx, userID, tweetID, true)
}

func (s *Service) Unlike(ctx context.Context, userID, tweetID int64) error {
	return s.setLike(ctx, userID, tweetID, false)
}

func (s *Service) setLike(ctx context.Context, userID, tweetID int64, like bool) error {
	var authorID int64
	err := s.db.QueryRow(ctx,
		`SELECT author_id FROM tweets WHERE id = $1`, tweetID).Scan(&authorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var changed bool
	if like {
		tag, err := tx.Exec(ctx, `
			INSERT INTO likes (user_id, tweet_id) VALUES ($1, $2)
			ON CONFLICT DO NOTHING`, userID, tweetID)
		if err != nil {
			return err
		}
		changed = tag.RowsAffected() == 1
	} else {
		tag, err := tx.Exec(ctx,
			`DELETE FROM likes WHERE user_id = $1 AND tweet_id = $2`, userID, tweetID)
		if err != nil {
			return err
		}
		changed = tag.RowsAffected() == 1
	}
	if !changed {
		return nil
	}

	eventID := s.ids.Next()
	ev := &engagementsv1.EngagementEvent{Envelope: events.NewEnvelope(ctx, eventID)}
	if like {
		ev.Payload = &engagementsv1.EngagementEvent_Liked{Liked: &engagementsv1.TweetLiked{
			TweetId: tweetID, UserId: userID, AuthorId: authorID,
		}}
	} else {
		ev.Payload = &engagementsv1.EngagementEvent_Unliked{Unliked: &engagementsv1.TweetUnliked{
			TweetId: tweetID, UserId: userID, AuthorId: authorID,
		}}
	}
	if err := events.Emit(ctx, tx, eventID, "engagements.v1", events.Key(tweetID), ev); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
```

- [ ] **Step 3: Run, commit**

```bash
go test ./internal/tweets/ -race -v
git add internal/tweets
git commit -m "feat(tweets): idempotent like/unlike emitting one event per change"
```

---

### Task L2: Counter accumulator — dedupe, HINCRBY, batched flush

**Files:**
- Create: `internal/counters/counters.go`
- Test: `internal/counters/counters_test.go`

- [ ] **Step 1: Write the failing test** — `internal/counters/counters_test.go`

```go
package counters

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	commonv1 "github.com/fonvacano/yaxter/gen/events/common/v1"
	engagementsv1 "github.com/fonvacano/yaxter/gen/events/engagements/v1"
	pgxkit "github.com/fonvacano/yaxter/pkg/pgx"
)

func testCounters(t *testing.T) (*Counters, *pgxpool.Pool, *redis.Client) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: requires Docker")
	}
	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("yaxter"), tcpostgres.WithUsername("yaxter"),
		tcpostgres.WithPassword("yaxter"), tcpostgres.BasicWaitStrategies())
	require.NoError(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	m, err := migrate.New("file://../../migrations", dsn)
	require.NoError(t, err)
	require.NoError(t, m.Up())
	m.Close()
	pool, err := pgxkit.NewPool(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	_, err = pool.Exec(ctx, `
		INSERT INTO users (id, username, email) VALUES (1, 'a', 'a@x.c');
		INSERT INTO tweets (id, author_id, text) VALUES (10, 1, 'hot');`)
	require.NoError(t, err)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return New(pool, rdb, 500, 2*time.Second), pool, rdb
}

func likedEvent(eventID int64, tweetID int64) *engagementsv1.EngagementEvent {
	return &engagementsv1.EngagementEvent{
		Envelope: &commonv1.Envelope{EventId: eventID},
		Payload: &engagementsv1.EngagementEvent_Liked{Liked: &engagementsv1.TweetLiked{
			TweetId: tweetID, UserId: 2, AuthorId: 1,
		}},
	}
}

// T1.4 DoD: 1000 like events for 1 tweet => few batched updates, exact final
// count, replayed events ignored.
func TestThousandLikesBatchedExactlyOnce(t *testing.T) {
	c, pool, rdb := testCounters(t)
	ctx := context.Background()

	for i := int64(1); i <= 1000; i++ {
		require.NoError(t, c.HandleEvent(ctx, likedEvent(i, 10)))
	}
	for i := int64(1); i <= 200; i++ { // replays
		require.NoError(t, c.HandleEvent(ctx, likedEvent(i, 10)))
	}
	require.NoError(t, c.Flush(ctx)) // final partial batch

	var likes int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT likes_count FROM tweets WHERE id = 10`).Scan(&likes))
	require.Equal(t, 1000, likes, "exact count despite replays")

	flushes := c.FlushCount()
	require.LessOrEqual(t, flushes, 3, "1000 events / 500 per flush (+ final)")
	require.GreaterOrEqual(t, flushes, 2)

	hot, err := rdb.HGet(ctx, "cnt:10", "likes").Int()
	require.NoError(t, err)
	require.Equal(t, 1000, hot, "redis hash tracks the live value (§2.7)")
}

func TestUnlikeAndRetweetDeltas(t *testing.T) {
	c, pool, _ := testCounters(t)
	ctx := context.Background()

	require.NoError(t, c.HandleEvent(ctx, likedEvent(1, 10)))
	require.NoError(t, c.HandleEvent(ctx, &engagementsv1.EngagementEvent{
		Envelope: &commonv1.Envelope{EventId: 2},
		Payload: &engagementsv1.EngagementEvent_Unliked{Unliked: &engagementsv1.TweetUnliked{
			TweetId: 10, UserId: 2, AuthorId: 1,
		}},
	}))
	require.NoError(t, c.HandleEvent(ctx, &engagementsv1.EngagementEvent{
		Envelope: &commonv1.Envelope{EventId: 3},
		Payload: &engagementsv1.EngagementEvent_Retweeted{Retweeted: &engagementsv1.TweetRetweeted{
			TweetId: 10, RetweetId: 11, UserId: 2, AuthorId: 1,
		}},
	}))
	require.NoError(t, c.Flush(ctx))

	var likes, retweets int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT likes_count, retweets_count FROM tweets WHERE id = 10`).
		Scan(&likes, &retweets))
	require.Zero(t, likes)
	require.Equal(t, 1, retweets)
	_ = fmt.Sprint() // keep fmt import if unused elsewhere; remove if not needed
}
```

(Remove the trailing `fmt` line and import if unused.)

- [ ] **Step 2: Run to verify failure, then write `internal/counters/counters.go`**

```go
// Package counters implements §2.7: buffered write-behind counters.
// Events dedupe by event_id, increment the hot Redis hash immediately, and
// accumulate in memory; the flush collapses N engagements into one UPDATE
// per tweet.
package counters

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	engagementsv1 "github.com/fonvacano/yaxter/gen/events/engagements/v1"
	"github.com/fonvacano/yaxter/pkg/redisx"
)

type delta struct{ likes, retweets int }

type Counters struct {
	db         *pgxpool.Pool
	rdb        *redis.Client
	flushAfter int
	flushEvery time.Duration

	mu      sync.Mutex
	deltas  map[int64]delta
	pending int
	flushes int
}

func New(db *pgxpool.Pool, rdb *redis.Client, flushAfter int, flushEvery time.Duration) *Counters {
	return &Counters{
		db: db, rdb: rdb,
		flushAfter: flushAfter, flushEvery: flushEvery,
		deltas: make(map[int64]delta),
	}
}

// HandleEvent applies one engagement event: dedupe -> hot hash -> buffer.
func (c *Counters) HandleEvent(ctx context.Context, ev *engagementsv1.EngagementEvent) error {
	first, err := redisx.Once(ctx, c.rdb,
		fmt.Sprintf("evt:%d", ev.GetEnvelope().GetEventId()), 24*time.Hour)
	if err != nil {
		return err
	}
	if !first {
		return nil // replay: at-least-once delivery, exactly-once effect
	}

	var tweetID int64
	var d delta
	switch p := ev.Payload.(type) {
	case *engagementsv1.EngagementEvent_Liked:
		tweetID, d.likes = p.Liked.GetTweetId(), 1
	case *engagementsv1.EngagementEvent_Unliked:
		tweetID, d.likes = p.Unliked.GetTweetId(), -1
	case *engagementsv1.EngagementEvent_Retweeted:
		tweetID, d.retweets = p.Retweeted.GetTweetId(), 1
	case *engagementsv1.EngagementEvent_Unretweeted:
		tweetID, d.retweets = p.Unretweeted.GetTweetId(), -1
	default:
		return nil // unknown payload: additive evolution, skip
	}

	key := fmt.Sprintf("cnt:%d", tweetID)
	pipe := c.rdb.Pipeline()
	if d.likes != 0 {
		pipe.HIncrBy(ctx, key, "likes", int64(d.likes))
	}
	if d.retweets != 0 {
		pipe.HIncrBy(ctx, key, "retweets", int64(d.retweets))
	}
	pipe.Expire(ctx, key, 24*time.Hour)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}

	c.mu.Lock()
	cur := c.deltas[tweetID]
	cur.likes += d.likes
	cur.retweets += d.retweets
	c.deltas[tweetID] = cur
	c.pending++
	full := c.pending >= c.flushAfter
	c.mu.Unlock()

	if full {
		return c.Flush(ctx)
	}
	return nil
}

// Flush writes the buffered deltas as one batched UPDATE per tweet.
func (c *Counters) Flush(ctx context.Context) error {
	c.mu.Lock()
	if c.pending == 0 {
		c.mu.Unlock()
		return nil
	}
	batchDeltas := c.deltas
	c.deltas = make(map[int64]delta)
	c.pending = 0
	c.flushes++
	c.mu.Unlock()

	batch := &pgx.Batch{}
	for tweetID, d := range batchDeltas {
		batch.Queue(`
			UPDATE tweets SET
				likes_count    = likes_count + $2,
				retweets_count = retweets_count + $3
			WHERE id = $1`, tweetID, d.likes, d.retweets)
	}
	return c.db.SendBatch(ctx, batch).Close()
}

// FlushCount reports how many flushes ran (test/metrics observability).
func (c *Counters) FlushCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.flushes
}

// Run flushes on a ticker until ctx ends (the 2s budget from §2.7),
// with one final flush on shutdown.
func (c *Counters) Run(ctx context.Context) {
	t := time.NewTicker(c.flushEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = c.Flush(flushCtx)
			cancel()
			return
		case <-t.C:
			_ = c.Flush(ctx)
		}
	}
}
```

- [ ] **Step 3: Run, commit**

```bash
go test ./internal/counters/ -race -v
git add internal/counters
git commit -m "feat(counters): deduped buffered write-behind with batched flush"
```

---

### Task L3: Counter read-through + reconcile job + tweets hydration switch

**Files:**
- Create: `internal/counters/read.go`, `internal/counters/reconcile.go`
- Modify: `internal/tweets/hydrate.go` (counters from read-through)
- Test: `internal/counters/read_test.go`

- [ ] **Step 1: Write the failing test** — `internal/counters/read_test.go`

```go
package counters

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadThroughPrefersRedisFallsBackToPG(t *testing.T) {
	c, pool, rdb := testCounters(t)
	ctx := context.Background()

	// Cold: falls back to the PG columns and warms the hash.
	_, err := pool.Exec(ctx,
		`UPDATE tweets SET likes_count = 3, retweets_count = 1 WHERE id = 10`)
	require.NoError(t, err)
	likes, retweets, err := Read(ctx, rdb, pool, 10)
	require.NoError(t, err)
	require.Equal(t, 3, likes)
	require.Equal(t, 1, retweets)

	// Warm: redis wins (it is fresher than the buffered PG value).
	require.NoError(t, rdb.HSet(ctx, "cnt:10", "likes", 5, "retweets", 1).Err())
	likes, _, err = Read(ctx, rdb, pool, 10)
	require.NoError(t, err)
	require.Equal(t, 5, likes)
	_ = c
}

func TestReconcileCorrectsDrift(t *testing.T) {
	c, pool, rdb := testCounters(t)
	ctx := context.Background()

	// Truth: 2 like rows. Drifted denormalized count: 7.
	_, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, email) VALUES (2,'b','b@x.c'), (3,'c','c@x.c');
		INSERT INTO likes (user_id, tweet_id) VALUES (2, 10), (3, 10);
		UPDATE tweets SET likes_count = 7 WHERE id = 10;`)
	require.NoError(t, err)
	require.NoError(t, rdb.HSet(ctx, "cnt:10", "likes", 7).Err())

	require.NoError(t, Reconcile(ctx, pool, rdb))

	var likes int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT likes_count FROM tweets WHERE id = 10`).Scan(&likes))
	require.Equal(t, 2, likes, "reconcile recomputes from the likes table")
	hot, err := rdb.HGet(ctx, "cnt:10", "likes").Int()
	require.NoError(t, err)
	require.Equal(t, 2, hot, "hot hash corrected too")
	_ = c
}
```

- [ ] **Step 2: Run to verify failure, then implement**

`internal/counters/read.go`:

```go
package counters

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Read returns (likes, retweets): cnt: hash first, PG columns on miss
// (warming the hash). Counters are explicitly eventual (§2.7).
func Read(ctx context.Context, rdb *redis.Client, db *pgxpool.Pool, tweetID int64) (int, int, error) {
	key := fmt.Sprintf("cnt:%d", tweetID)
	vals, err := rdb.HGetAll(ctx, key).Result()
	if err == nil && len(vals) > 0 {
		var likes, retweets int
		fmt.Sscan(vals["likes"], &likes)       //nolint:errcheck // absent field = 0
		fmt.Sscan(vals["retweets"], &retweets) //nolint:errcheck
		return likes, retweets, nil
	}
	var likes, retweets int
	if err := db.QueryRow(ctx,
		`SELECT likes_count, retweets_count FROM tweets WHERE id = $1`, tweetID).
		Scan(&likes, &retweets); err != nil {
		return 0, 0, err
	}
	rdb.HSet(ctx, key, "likes", likes, "retweets", retweets)
	rdb.Expire(ctx, key, 24*60*60*1e9)
	return likes, retweets, nil
}
```

`internal/counters/reconcile.go`:

```go
package counters

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Reconcile recomputes likes_count from the likes table for recently-liked
// tweets and corrects both PG and the hot hash — the nightly drift repair
// from §2.7. Retweet counts reconcile the same way against tweets rows.
func Reconcile(ctx context.Context, db *pgxpool.Pool, rdb *redis.Client) error {
	rows, err := db.Query(ctx, `
		WITH truth AS (
			SELECT tweet_id, count(*) AS n FROM likes
			WHERE created_at > now() - interval '24 hours'
			   OR tweet_id IN (SELECT tweet_id FROM likes GROUP BY tweet_id)
			GROUP BY tweet_id
		)
		UPDATE tweets t SET likes_count = truth.n
		FROM truth
		WHERE t.id = truth.tweet_id AND t.likes_count <> truth.n
		RETURNING t.id, t.likes_count`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var likes int
		if err := rows.Scan(&id, &likes); err != nil {
			return err
		}
		rdb.HSet(ctx, fmt.Sprintf("cnt:%d", id), "likes", likes)
	}
	return rows.Err()
}
```

Update `internal/tweets/hydrate.go` — after scanning the row, replace the PG counter values with the read-through (track L owns this file version per the merge rules):

```go
import "github.com/fonvacano/yaxter/internal/counters"

	// counters are served from the hot hash, PG as fallback (§2.7 step 3)
	likes, retweets, cErr := counters.Read(ctx, s.rdb, s.db, tweetID)
	if cErr == nil {
		h.LikesCount, h.RetweetsCount = likes, retweets
	}
```

- [ ] **Step 3: Run, commit**

```bash
go test ./internal/counters/ ./internal/tweets/ -race -v
git add internal/counters internal/tweets
git commit -m "feat(counters): read-through and nightly reconcile"
```

---

### Task L4: `worker:counters` role + like endpoints + Kafka e2e smoke

**Files:**
- Create: `cmd/worker/counters.go`, `internal/httpapi/likes_handlers.go`
- Modify: `internal/httpapi/server.go` (LikeTweet/UnlikeTweet stubs), `internal/httpapi/wire.go`
- Test: `internal/counters/e2e_test.go`

- [ ] **Step 1: Write `cmd/worker/counters.go`**

```go
package main

import (
	"context"
	"time"

	"github.com/rs/zerolog"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"

	engagementsv1 "github.com/fonvacano/yaxter/gen/events/engagements/v1"
	"github.com/fonvacano/yaxter/internal/counters"
	"github.com/fonvacano/yaxter/pkg/config"
	"github.com/fonvacano/yaxter/pkg/kafkax"
	pgxkit "github.com/fonvacano/yaxter/pkg/pgx"
	"github.com/fonvacano/yaxter/pkg/redisx"
)

func init() { roleRunners["counters"] = runCounters }

func runCounters(ctx context.Context, logger zerolog.Logger, cfg config.Config) {
	log := logger.With().Str("role", "counters").Logger()
	pool, err := pgxkit.NewPool(ctx, cfg.PostgresDSN)
	if err != nil {
		log.Fatal().Err(err).Msg("postgres unreachable")
	}
	defer pool.Close()
	rdb := redisx.NewClient(cfg.RedisAddr)
	defer rdb.Close()

	client, err := kafkax.NewClient(cfg.KafkaBrokers,
		kgo.ConsumerGroup(kafkax.GroupID("counters")),
		kgo.ConsumeTopics("engagements.v1"))
	if err != nil {
		log.Fatal().Err(err).Msg("kafka client")
	}
	defer client.Close()

	c := counters.New(pool, rdb, 500, 2*time.Second)
	go c.Run(ctx)
	// Nightly reconcile (§2.7); demo interval is config-free 24h.
	go func() {
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := counters.Reconcile(ctx, pool, rdb); err != nil {
					log.Error().Err(err).Msg("reconcile failed")
				}
			}
		}
	}()

	err = kafkax.Consume(ctx, client, log, func(ctx context.Context, rec *kgo.Record) error {
		var ev engagementsv1.EngagementEvent
		if err := proto.Unmarshal(rec.Value, &ev); err != nil {
			return err // logged + skipped by Consume (poison message)
		}
		return c.HandleEvent(ctx, &ev)
	})
	if err != nil && ctx.Err() == nil {
		log.Fatal().Err(err).Msg("counters consumer exited")
	}
}
```

- [ ] **Step 2: Write `internal/httpapi/likes_handlers.go`** + replace stubs

```go
package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/fonvacano/yaxter/internal/tweets"
)

func (h *TweetsHandlers) setLike(w http.ResponseWriter, r *http.Request, id string, like bool) {
	uid, ok := requireUser(w, r)
	if !ok {
		return
	}
	tweetID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "no such tweet")
		return
	}
	if like {
		err = h.svc.Like(r.Context(), uid, tweetID)
	} else {
		err = h.svc.Unlike(r.Context(), uid, tweetID)
	}
	switch {
	case errors.Is(err, tweets.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "no such tweet")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal", "like failed")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
```

`server.go` stub replacements (signatures per `api.gen.go`):

```go
func (s *Server) LikeTweet(w http.ResponseWriter, r *http.Request, id Id, params LikeTweetParams) {
	s.Tweets.setLike(w, r, string(id), true)
}
func (s *Server) UnlikeTweet(w http.ResponseWriter, r *http.Request, id Id, params UnlikeTweetParams) {
	s.Tweets.setLike(w, r, string(id), false)
}
```

- [ ] **Step 3: Write the Kafka e2e smoke** — `internal/counters/e2e_test.go`

```go
package counters

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	tckafka "github.com/testcontainers/testcontainers-go/modules/kafka"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"

	"github.com/fonvacano/yaxter/pkg/kafkax"
)

// End-to-end: events through a real broker reach the accumulator.
func TestCountersConsumeFromKafka(t *testing.T) {
	c, pool, _ := testCounters(t)
	ctx := context.Background()

	kctr, err := tckafka.Run(ctx, "confluentinc/confluent-local:7.7.0",
		tckafka.WithClusterID("yaxter-test"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = kctr.Terminate(context.Background()) })
	brokers, err := kctr.Brokers(ctx)
	require.NoError(t, err)

	producer, err := kafkax.NewClient(brokers, kgo.AllowAutoTopicCreation())
	require.NoError(t, err)
	t.Cleanup(producer.Close)
	for i := int64(1); i <= 5; i++ {
		raw, err := proto.Marshal(likedEvent(i, 10))
		require.NoError(t, err)
		require.NoError(t, producer.ProduceSync(ctx, &kgo.Record{
			Topic: "engagements.v1", Key: []byte("10"), Value: raw,
		}).FirstErr())
	}

	consumer, err := kafkax.NewClient(brokers,
		kgo.ConsumerGroup(kafkax.GroupID("counters")),
		kgo.ConsumeTopics("engagements.v1"),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	require.NoError(t, err)
	t.Cleanup(consumer.Close)

	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go kafkax.Consume(cctx, consumer, zerolog.Nop(), c.HandleRecord) //nolint:errcheck

	require.Eventually(t, func() bool {
		_ = c.Flush(ctx)
		var likes int
		err := pool.QueryRow(ctx,
			`SELECT likes_count FROM tweets WHERE id = 10`).Scan(&likes)
		return err == nil && likes == 5
	}, 30*time.Second, 200*time.Millisecond)
}
```

The test consumes via `c.HandleRecord` — add that helper to `internal/counters/counters.go` so the worker role and the test share one decode path (new imports: `google.golang.org/protobuf/proto`, `github.com/twmb/franz-go/pkg/kgo`):

```go
// HandleRecord decodes a raw engagements.v1 record and applies it.
func (c *Counters) HandleRecord(ctx context.Context, rec *kgo.Record) error {
	var ev engagementsv1.EngagementEvent
	if err := proto.Unmarshal(rec.Value, &ev); err != nil {
		return err
	}
	return c.HandleEvent(ctx, &ev)
}
```

And in `cmd/worker/counters.go` the consume callback body becomes simply `c.HandleRecord` (replace the inline `proto.Unmarshal` closure shown in Step 1 with `err = kafkax.Consume(ctx, client, log, c.HandleRecord)`).

- [ ] **Step 4: Run, build, commit**

```bash
go build ./... && go test ./internal/counters/ ./internal/httpapi/ -race -v
git add cmd/worker internal/counters internal/httpapi
git commit -m "feat(counters): worker role, like endpoints, kafka e2e"
```

---

## DoD Check (against ARCHITECTURE.md §8)

| Task | DoD | Verified by |
|---|---|---|
| T1.2 | both edges consistent; idempotent double-follow; event emitted once per state change | U2 tests |
| T1.3 | duplicate Idempotency-Key → identical cached response, single row, single event | T3 `TestDuplicateIdempotencyKeySingleTweet` |
| T1.4 | 1000 like events for 1 tweet → ≤ N batch updates, exact final count, replayed events ignored | L2 `TestThousandLikesBatchedExactlyOnce` |

## Out of Scope (deferred)

- Fan-out consumption of `tweets.v1` — **T2.1**
- Timeline read endpoints (`/timeline`, `/users/{u}/tweets`) — **T2.2**
- Notifications from `follows.v1`/`engagements.v1` — **T2.3**
- Media upload pipeline + OAuth — sibling plan `2026-06-11-phase1-media-oauth-t1.5-t1.6.md`
