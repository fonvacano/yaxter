# Phase 0 Remainder (T0.2–T0.5) — Contracts & Scaffolding Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete Phase 0 of ARCHITECTURE.md §8: the OpenAPI contract + generated stubs/client (T0.2), protobuf event contracts with buf gates (T0.3), all DB migrations plus the 256-logical-shard routing kit and outbox helper (T0.4), and the docker-compose dev stack with seed harness (T0.5).

**Architecture:** Contracts-first. `api/openapi.yaml` is the single source of truth for HTTP (server stubs → `internal/httpapi`, Go client → `pkg/apiclient`); `proto/events/**` is the registry for Kafka payloads (generated to `gen/`); `migrations/` defines the §2.2 schema; `pkg/sharding` + `pkg/outbox` are the data-access kit every Phase 1 task builds on. T0.5 wires it all into `make up`.

**Tech Stack:** OpenAPI 3.0.3 + spectral + oapi-codegen v2 (std-http server, client), protobuf + buf v2 (remote protoc-gen-go), golang-migrate v4, pgx/v5, gopkg.in/yaml.v3, docker compose (PG16, Redis 7, Kafka 3.8 KRaft, MinIO, Jaeger, navikt mock-oauth2-server).

**Prerequisite:** T0.1 plan executed (`docs/superpowers/plans/2026-06-10-t0.1-repo-scaffold.md`) — module `github.com/fonvacano/yaxter`, `pkg/{config,log,otel,snowflake,pgx,redisx,idem,kafkax}` exist, CI green.

**Deliberate deviations from ARCHITECTURE.md (record here, not silently):**
1. Spec header is `openapi: 3.0.3`, not 3.1 — oapi-codegen (kin-openapi) does not fully support 3.1. Contract content is unaffected; revisit when the toolchain catches up.
2. Repository interfaces (mentioned in T0.4 scope) are deferred to their first Phase 1 implementations — defining them with zero implementations invites drift (YAGNI). The shard router and outbox helper *are* delivered here.
3. `Idempotency-Key` is required on domain mutations (tweets, likes, follows, media, profile, register, notifications/read) but **not** on login/refresh/logout/OAuth — token issuance is protected by rate limits and OAuth `state` instead; replaying a login is not a duplicate-resource hazard.

---

## File Structure

```
yaxter/
├── api/
│   ├── openapi.yaml               # the HTTP contract               (Tasks 1–4)
│   ├── server.cfg.yaml            # oapi-codegen config             (Task 5)
│   └── client.cfg.yaml            # oapi-codegen config             (Task 5)
├── .spectral.yaml                                                    (Task 1)
├── internal/httpapi/api.gen.go    # generated server stubs          (Task 5)
├── pkg/apiclient/client.gen.go    # generated client (seed/k6)      (Task 5)
├── buf.yaml / buf.gen.yaml                                           (Task 6)
├── proto/events/
│   ├── common/v1/envelope.proto                                      (Task 6)
│   ├── tweets/v1/events.proto                                        (Task 6)
│   ├── engagements/v1/events.proto                                   (Task 6)
│   ├── follows/v1/events.proto                                       (Task 6)
│   └── media/v1/events.proto                                         (Task 6)
├── gen/events/**                  # generated Go from protos         (Task 6)
├── docs/events.md                 # topic/key/ordering contract      (Task 7)
├── migrations/                    # golang-migrate SQL pairs         (Task 8)
├── pkg/sharding/
│   ├── shard.go                   # LogicalShard + Map               (Task 9)
│   ├── config.go                  # YAML shard-map config            (Task 9)
│   └── router.go                  # pgx pools + ForEachShard         (Task 10)
├── pkg/outbox/outbox.go           # same-tx insert helper            (Task 11)
├── configs/shardmap.yaml          # demo: 256 logicals → 1 cluster   (Task 10)
├── docker-compose.yaml                                               (Task 12)
└── cmd/seed/main.go               # harness skeleton                 (Task 13)
```

---

## Parallel Execution Map (subagent dispatch)

The user wants this plan executed by **parallel subagents**. Prerequisite: the entire T0.1 plan is merged first — every track imports its packages.

| Track | Tasks (in order) | Depends on tracks | Shared files touched |
|---|---|---|---|
| A — HTTP contract | 1 → 2 → 3 → 4 → 5 | — | `Makefile` (Tasks 1, 5), `go.mod` |
| B — Event contracts | 6 → 7 | — | `go.mod` |
| C1 — Migrations | 8 | — | `go.mod` |
| C2 — Sharding kit | 9 → 10 | — | `go.mod` |
| C3 — Outbox | 11 | C1 | — |
| D — Dev stack | 12 | C1 | `Makefile`, `docker-compose.yaml` |
| E — Seed + CI gates | 13 | A, B, D | `Makefile`, `.github/workflows/ci.yaml` |

**Rules for parallel dispatch:**

1. Wave 1: A, B, C1, C2 in parallel. Wave 2: C3, D in parallel (after C1). Wave 3: E (after A, B, D).
2. One track per subagent, each in its **own git worktree**; never two subagents in one worktree. Merge tracks back in the order A, B, C1, C2, C3, D, E.
3. `go.mod`/`go.sum` merge conflicts: accept both sides, then `go mod tidy && go test -short ./...` before completing the merge.
4. `Makefile` conflicts: the targets are disjoint by design — union them (and union the `.PHONY` lists).
5. Each subagent runs its track's tests before its final commit; the merger re-runs `make test` after every merge.

---

### Task 1: OpenAPI skeleton — error model, security, shared components, spectral

**Files:**
- Create: `api/openapi.yaml`, `.spectral.yaml`
- Modify: `Makefile` (add `lint-spec`)

- [ ] **Step 1: Create `.spectral.yaml`**

```yaml
extends: ["spectral:oas"]
```

- [ ] **Step 2: Create `api/openapi.yaml`** with everything except `paths` (added in Tasks 2–4)

```yaml
openapi: 3.0.3
info:
  title: Yaxter API
  version: 0.1.0
  description: |
    Twitter-like demo API (ARCHITECTURE.md §1.2). This file is the source of
    truth: Go server stubs, the Go client (seed/k6), and the SPA client
    (orval, T1.7) are generated from it. All IDs are 64-bit snowflakes
    serialized as decimal strings.
tags:
  - name: auth
  - name: users
  - name: tweets
  - name: timeline
  - name: notifications
  - name: media
servers:
  - url: /v1
security:
  - bearerAuth: []
paths: {}
components:
  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT
  parameters:
    IdempotencyKey:
      name: Idempotency-Key
      in: header
      required: true
      description: UUID dedupe key; 24h window (ARCHITECTURE.md §7).
      schema:
        type: string
        format: uuid
    Cursor:
      name: cursor
      in: query
      description: Opaque cursor from the previous page's next_cursor.
      schema:
        type: string
    Limit:
      name: limit
      in: query
      schema:
        type: integer
        minimum: 1
        maximum: 100
        default: 20
    Username:
      name: username
      in: path
      required: true
      schema:
        type: string
        pattern: '^[A-Za-z0-9_]{3,30}$'
    TweetId:
      name: id
      in: path
      required: true
      schema:
        $ref: '#/components/schemas/Id'
    MediaId:
      name: id
      in: path
      required: true
      schema:
        $ref: '#/components/schemas/Id'
    Provider:
      name: provider
      in: path
      required: true
      description: Enabled set is config-driven; discover via /auth/providers.
      schema:
        type: string
        enum: [yandex, google]
  schemas:
    Id:
      type: string
      pattern: '^[0-9]+$'
      description: Snowflake ID as a decimal string (int64-safe in JSON).
    Error:
      type: object
      required: [error, message]
      properties:
        error:
          type: string
          description: Machine-readable code, e.g. validation_failed.
        message:
          type: string
  responses:
    BadRequest:
      description: Malformed request or validation failure.
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/Error'
    Unauthorized:
      description: Missing/invalid/expired credentials.
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/Error'
    Forbidden:
      description: Authenticated but not allowed (ownership checks).
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/Error'
    NotFound:
      description: Resource does not exist (uniform — no enumeration leaks).
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/Error'
    Conflict:
      description: State conflict (duplicate, in-flight idempotent request).
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/Error'
    TooManyRequests:
      description: Rate limited.
      headers:
        Retry-After:
          schema:
            type: integer
          description: Seconds until the window frees up.
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/Error'
```

- [ ] **Step 3: Add `lint-spec` to `Makefile`**

```makefile
lint-spec:
	npx -y @stoplight/spectral-cli lint --fail-severity=error api/openapi.yaml
```

(Append to the existing `.PHONY` line: `lint-spec`.)

- [ ] **Step 4: Verify the skeleton lints**

Run: `make lint-spec`
Expected: exit 0 (warnings allowed, zero errors).

- [ ] **Step 5: Commit**

```bash
git add api/openapi.yaml .spectral.yaml Makefile
git commit -m "feat(api): openapi skeleton - error model, security, shared components"
```

---

### Task 2: OpenAPI — auth + OAuth endpoints

**Files:**
- Modify: `api/openapi.yaml`

- [ ] **Step 1: Add auth schemas under `components.schemas`**

```yaml
    RegisterRequest:
      type: object
      required: [username, email, password]
      properties:
        username:
          type: string
          pattern: '^[A-Za-z0-9_]{3,30}$'
        email:
          type: string
          format: email
        password:
          type: string
          minLength: 8
          maxLength: 128
    LoginRequest:
      type: object
      required: [login, password]
      properties:
        login:
          type: string
          description: Username or email.
        password:
          type: string
    RefreshRequest:
      type: object
      properties:
        refresh_token:
          type: string
          description: Omit when the HttpOnly refresh cookie is present (web).
    TokenPair:
      type: object
      required: [access_token, token_type, expires_in]
      properties:
        access_token:
          type: string
        token_type:
          type: string
          enum: [Bearer]
        expires_in:
          type: integer
          description: Access-token lifetime in seconds (900 = 15 min).
        refresh_token:
          type: string
          description: Returned in the body for non-web clients; web clients get a SameSite=Strict HttpOnly cookie instead.
    UserSummary:
      type: object
      required: [id, username]
      properties:
        id:
          $ref: '#/components/schemas/Id'
        username:
          type: string
        avatar_url:
          type: string
          nullable: true
    User:
      type: object
      required: [id, username, bio, followers_count, following_count, created_at]
      properties:
        id:
          $ref: '#/components/schemas/Id'
        username:
          type: string
        bio:
          type: string
        avatar_url:
          type: string
          nullable: true
        followers_count:
          type: integer
        following_count:
          type: integer
        created_at:
          type: string
          format: date-time
    PrivateUser:
      allOf:
        - $ref: '#/components/schemas/User'
        - type: object
          required: [email, has_password, linked_providers]
          properties:
            email:
              type: string
              format: email
            has_password:
              type: boolean
              description: false for OAuth-only accounts (pass_hash NULL).
            linked_providers:
              type: array
              items:
                type: string
    AuthResponse:
      type: object
      required: [user, tokens]
      properties:
        user:
          $ref: '#/components/schemas/PrivateUser'
        tokens:
          $ref: '#/components/schemas/TokenPair'
    OAuthProviderInfo:
      type: object
      required: [name, display_name, start_url]
      properties:
        name:
          type: string
        display_name:
          type: string
        start_url:
          type: string
    ProviderList:
      type: object
      required: [providers]
      properties:
        providers:
          type: array
          items:
            $ref: '#/components/schemas/OAuthProviderInfo'
    LinkStartResponse:
      type: object
      required: [auth_url]
      properties:
        auth_url:
          type: string
          description: Provider URL to complete linking in the browser.
```

- [ ] **Step 2: Replace `paths: {}` with the auth paths**

```yaml
paths:
  /auth/register:
    post:
      tags: [auth]
      summary: Create an account
      operationId: register
      security: []
      parameters:
        - $ref: '#/components/parameters/IdempotencyKey'
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/RegisterRequest'
      responses:
        '201':
          description: Account created; standard token pair issued.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/AuthResponse'
        '400':
          $ref: '#/components/responses/BadRequest'
        '409':
          $ref: '#/components/responses/Conflict'
        '429':
          $ref: '#/components/responses/TooManyRequests'
  /auth/login:
    post:
      tags: [auth]
      summary: Log in with username/email + password
      operationId: login
      security: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/LoginRequest'
      responses:
        '200':
          description: Token pair issued. OAuth-only accounts (no password) get a uniform 401.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/AuthResponse'
        '401':
          $ref: '#/components/responses/Unauthorized'
        '429':
          $ref: '#/components/responses/TooManyRequests'
  /auth/refresh:
    post:
      tags: [auth]
      summary: Rotate the refresh token, issue a new pair
      operationId: refreshToken
      security: []
      requestBody:
        required: false
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/RefreshRequest'
      responses:
        '200':
          description: New pair; old refresh token is revoked (rotation). Reuse of a rotated token revokes the whole family.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/TokenPair'
        '401':
          $ref: '#/components/responses/Unauthorized'
  /auth/logout:
    post:
      tags: [auth]
      summary: Revoke the current refresh-token family
      operationId: logout
      responses:
        '204':
          description: Logged out.
        '401':
          $ref: '#/components/responses/Unauthorized'
  /auth/providers:
    get:
      tags: [auth]
      summary: List enabled OAuth providers (config-driven)
      operationId: listAuthProviders
      security: []
      responses:
        '200':
          description: Enabled providers; clients render only these buttons.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ProviderList'
  /auth/oauth/{provider}/start:
    get:
      tags: [auth]
      summary: Begin the authorization-code + PKCE dance
      operationId: oauthStart
      security: []
      parameters:
        - $ref: '#/components/parameters/Provider'
        - name: redirect_to
          in: query
          description: SPA route to return to after callback (validated allowlist).
          schema:
            type: string
      responses:
        '302':
          description: Redirect to the provider with state + PKCE challenge.
          headers:
            Location:
              schema:
                type: string
        '404':
          $ref: '#/components/responses/NotFound'
        '429':
          $ref: '#/components/responses/TooManyRequests'
  /auth/oauth/{provider}/callback:
    get:
      tags: [auth]
      summary: Provider callback — exchange code, upsert identity, issue tokens
      operationId: oauthCallback
      security: []
      parameters:
        - $ref: '#/components/parameters/Provider'
        - name: code
          in: query
          required: true
          schema:
            type: string
        - name: state
          in: query
          required: true
          schema:
            type: string
      responses:
        '200':
          description: Same shape as login. State is single-use; replays get 400.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/AuthResponse'
        '400':
          $ref: '#/components/responses/BadRequest'
        '404':
          $ref: '#/components/responses/NotFound'
        '429':
          $ref: '#/components/responses/TooManyRequests'
  /auth/oauth/{provider}/link:
    post:
      tags: [auth]
      summary: Start linking a provider to the authenticated account
      operationId: oauthLink
      parameters:
        - $ref: '#/components/parameters/Provider'
      responses:
        '200':
          description: URL to complete linking at the provider.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/LinkStartResponse'
        '401':
          $ref: '#/components/responses/Unauthorized'
        '404':
          $ref: '#/components/responses/NotFound'
        '409':
          $ref: '#/components/responses/Conflict'
    delete:
      tags: [auth]
      summary: Unlink a provider
      operationId: oauthUnlink
      parameters:
        - $ref: '#/components/parameters/Provider'
      responses:
        '204':
          description: Unlinked.
        '401':
          $ref: '#/components/responses/Unauthorized'
        '404':
          $ref: '#/components/responses/NotFound'
        '409':
          description: Refused — unlinking would leave the account with no credential (OAuth-only account, no password).
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Error'
```

- [ ] **Step 3: Lint**

Run: `make lint-spec`
Expected: exit 0.

- [ ] **Step 4: Commit**

```bash
git add api/openapi.yaml
git commit -m "feat(api): auth and oauth endpoints in openapi contract"
```

---

### Task 3: OpenAPI — users, follows, tweets, likes

**Files:**
- Modify: `api/openapi.yaml`

- [ ] **Step 1: Add schemas under `components.schemas`**

```yaml
    UpdateProfileRequest:
      type: object
      properties:
        bio:
          type: string
          maxLength: 160
        avatar_media_id:
          $ref: '#/components/schemas/Id'
    MediaRef:
      type: object
      required: [id, urls]
      properties:
        id:
          $ref: '#/components/schemas/Id'
        urls:
          type: object
          required: [thumb, feed, orig]
          description: Fixed scheme https://media.{domain}/{variant}/{media_id}.webp (§2.5).
          properties:
            thumb:
              type: string
            feed:
              type: string
            orig:
              type: string
    CreateTweetRequest:
      type: object
      required: [text]
      properties:
        text:
          type: string
          maxLength: 280
          description: May be empty only when retweet_of_id is set.
        media_ids:
          type: array
          maxItems: 4
          items:
            $ref: '#/components/schemas/Id'
          description: Each must be in `ready` state.
        retweet_of_id:
          $ref: '#/components/schemas/Id'
    Tweet:
      type: object
      required: [id, author, text, likes_count, retweets_count, created_at]
      properties:
        id:
          $ref: '#/components/schemas/Id'
        author:
          $ref: '#/components/schemas/UserSummary'
        text:
          type: string
        retweet_of:
          allOf:
            - $ref: '#/components/schemas/Tweet'
          nullable: true
          description: One level deep — retweets of retweets are flattened to the original.
        media:
          type: array
          items:
            $ref: '#/components/schemas/MediaRef'
        likes_count:
          type: integer
          description: Eventual (±seconds), from the counter pipeline (§2.7).
        retweets_count:
          type: integer
        liked_by_me:
          type: boolean
          description: Present only on authenticated reads.
        created_at:
          type: string
          format: date-time
    TweetPage:
      type: object
      required: [items]
      properties:
        items:
          type: array
          items:
            $ref: '#/components/schemas/Tweet'
        next_cursor:
          type: string
          nullable: true
          description: Absent/null on the last page.
    UserPage:
      type: object
      required: [items]
      properties:
        items:
          type: array
          items:
            $ref: '#/components/schemas/UserSummary'
        next_cursor:
          type: string
          nullable: true
```

- [ ] **Step 2: Add paths**

```yaml
  /users/me:
    get:
      tags: [users]
      summary: Current user's profile (read-your-writes)
      operationId: getMe
      responses:
        '200':
          description: Profile.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/PrivateUser'
        '401':
          $ref: '#/components/responses/Unauthorized'
    patch:
      tags: [users]
      summary: Update profile
      operationId: updateMe
      parameters:
        - $ref: '#/components/parameters/IdempotencyKey'
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/UpdateProfileRequest'
      responses:
        '200':
          description: Updated profile (cache deleted on write).
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/PrivateUser'
        '400':
          $ref: '#/components/responses/BadRequest'
        '401':
          $ref: '#/components/responses/Unauthorized'
  /users/{username}:
    get:
      tags: [users]
      summary: Public profile
      operationId: getUser
      security: []
      parameters:
        - $ref: '#/components/parameters/Username'
      responses:
        '200':
          description: Profile.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/User'
        '404':
          $ref: '#/components/responses/NotFound'
  /users/{username}/follow:
    post:
      tags: [users]
      summary: Follow a user (idempotent; writes both edge tables + event in one tx)
      operationId: followUser
      parameters:
        - $ref: '#/components/parameters/Username'
        - $ref: '#/components/parameters/IdempotencyKey'
      responses:
        '204':
          description: Following (already-following is a no-op 204).
        '400':
          description: Self-follow rejected.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Error'
        '401':
          $ref: '#/components/responses/Unauthorized'
        '404':
          $ref: '#/components/responses/NotFound'
        '429':
          $ref: '#/components/responses/TooManyRequests'
    delete:
      tags: [users]
      summary: Unfollow a user (idempotent)
      operationId: unfollowUser
      parameters:
        - $ref: '#/components/parameters/Username'
        - $ref: '#/components/parameters/IdempotencyKey'
      responses:
        '204':
          description: Not following anymore.
        '401':
          $ref: '#/components/responses/Unauthorized'
        '404':
          $ref: '#/components/responses/NotFound'
  /users/{username}/followers:
    get:
      tags: [users]
      summary: Who follows this user
      operationId: listFollowers
      security: []
      parameters:
        - $ref: '#/components/parameters/Username'
        - $ref: '#/components/parameters/Cursor'
        - $ref: '#/components/parameters/Limit'
      responses:
        '200':
          description: Page of followers.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/UserPage'
        '404':
          $ref: '#/components/responses/NotFound'
  /users/{username}/following:
    get:
      tags: [users]
      summary: Who this user follows
      operationId: listFollowing
      security: []
      parameters:
        - $ref: '#/components/parameters/Username'
        - $ref: '#/components/parameters/Cursor'
        - $ref: '#/components/parameters/Limit'
      responses:
        '200':
          description: Page of followees.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/UserPage'
        '404':
          $ref: '#/components/responses/NotFound'
  /tweets:
    post:
      tags: [tweets]
      summary: Create a tweet or retweet (retweet_of_id set)
      operationId: createTweet
      parameters:
        - $ref: '#/components/parameters/IdempotencyKey'
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/CreateTweetRequest'
      responses:
        '201':
          description: Acked once the row + outbox event are committed (§2.4). Duplicate Idempotency-Key replays this exact response.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Tweet'
        '400':
          $ref: '#/components/responses/BadRequest'
        '401':
          $ref: '#/components/responses/Unauthorized'
        '404':
          description: retweet_of_id or a media_id does not exist / is not ready.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Error'
        '429':
          $ref: '#/components/responses/TooManyRequests'
  /tweets/{id}:
    get:
      tags: [tweets]
      summary: Fetch one tweet
      operationId: getTweet
      security: []
      parameters:
        - $ref: '#/components/parameters/TweetId'
      responses:
        '200':
          description: The tweet.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Tweet'
        '404':
          $ref: '#/components/responses/NotFound'
    delete:
      tags: [tweets]
      summary: Delete own tweet
      operationId: deleteTweet
      parameters:
        - $ref: '#/components/parameters/TweetId'
      responses:
        '204':
          description: Deleted; TweetDeleted event emitted via outbox.
        '401':
          $ref: '#/components/responses/Unauthorized'
        '403':
          $ref: '#/components/responses/Forbidden'
        '404':
          $ref: '#/components/responses/NotFound'
  /tweets/{id}/like:
    post:
      tags: [tweets]
      summary: Like (idempotent — PK ON CONFLICT DO NOTHING)
      operationId: likeTweet
      parameters:
        - $ref: '#/components/parameters/TweetId'
        - $ref: '#/components/parameters/IdempotencyKey'
      responses:
        '204':
          description: Liked.
        '401':
          $ref: '#/components/responses/Unauthorized'
        '404':
          $ref: '#/components/responses/NotFound'
        '429':
          $ref: '#/components/responses/TooManyRequests'
    delete:
      tags: [tweets]
      summary: Unlike (idempotent)
      operationId: unlikeTweet
      parameters:
        - $ref: '#/components/parameters/TweetId'
        - $ref: '#/components/parameters/IdempotencyKey'
      responses:
        '204':
          description: Not liked anymore.
        '401':
          $ref: '#/components/responses/Unauthorized'
        '404':
          $ref: '#/components/responses/NotFound'
```

- [ ] **Step 3: Lint**

Run: `make lint-spec`
Expected: exit 0.

- [ ] **Step 4: Commit**

```bash
git add api/openapi.yaml
git commit -m "feat(api): users, follows, tweets, likes endpoints"
```

---

### Task 4: OpenAPI — timelines, notifications, media

**Files:**
- Modify: `api/openapi.yaml`

- [ ] **Step 1: Add schemas under `components.schemas`**

```yaml
    Notification:
      type: object
      required: [id, kind, actor, created_at, read]
      properties:
        id:
          $ref: '#/components/schemas/Id'
        kind:
          type: string
          enum: [follow, like, retweet, oauth_link]
        actor:
          $ref: '#/components/schemas/UserSummary'
        subject_id:
          allOf:
            - $ref: '#/components/schemas/Id'
          nullable: true
          description: Tweet ID for like/retweet; null for follow/oauth_link.
        created_at:
          type: string
          format: date-time
        read:
          type: boolean
    NotificationPage:
      type: object
      required: [items]
      properties:
        items:
          type: array
          items:
            $ref: '#/components/schemas/Notification'
        next_cursor:
          type: string
          nullable: true
    UnreadCount:
      type: object
      required: [count]
      properties:
        count:
          type: integer
    MarkReadRequest:
      type: object
      required: [up_to_id]
      properties:
        up_to_id:
          $ref: '#/components/schemas/Id'
    CreateMediaRequest:
      type: object
      required: [content_type, size_bytes]
      properties:
        content_type:
          type: string
          enum: [image/jpeg, image/png, image/webp]
        size_bytes:
          type: integer
          minimum: 1
          maximum: 5242880
    MediaUploadTicket:
      type: object
      required: [media_id, upload_url, expires_at]
      properties:
        media_id:
          $ref: '#/components/schemas/Id'
        upload_url:
          type: string
          description: Pre-signed PUT, scoped to exact key + content-length-range, 5-min expiry.
        expires_at:
          type: string
          format: date-time
    Media:
      type: object
      required: [id, status]
      properties:
        id:
          $ref: '#/components/schemas/Id'
        status:
          type: string
          enum: [pending, uploaded, ready, failed]
        urls:
          type: object
          nullable: true
          description: Populated when status=ready.
          properties:
            thumb:
              type: string
            feed:
              type: string
            orig:
              type: string
```

- [ ] **Step 2: Add paths**

```yaml
  /timeline:
    get:
      tags: [timeline]
      summary: Home timeline (hybrid fan-out merge, §2.1; eventual <5s)
      operationId: getHomeTimeline
      parameters:
        - $ref: '#/components/parameters/Cursor'
        - $ref: '#/components/parameters/Limit'
      responses:
        '200':
          description: Reverse-chron page; cursor is stable across new inserts (snowflake-based).
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/TweetPage'
        '401':
          $ref: '#/components/responses/Unauthorized'
  /users/{username}/tweets:
    get:
      tags: [timeline]
      summary: Profile timeline
      operationId: getUserTweets
      security: []
      parameters:
        - $ref: '#/components/parameters/Username'
        - $ref: '#/components/parameters/Cursor'
        - $ref: '#/components/parameters/Limit'
      responses:
        '200':
          description: The user's tweets, newest first.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/TweetPage'
        '404':
          $ref: '#/components/responses/NotFound'
  /notifications:
    get:
      tags: [notifications]
      summary: List notifications (eventual, <30s)
      operationId: listNotifications
      parameters:
        - $ref: '#/components/parameters/Cursor'
        - $ref: '#/components/parameters/Limit'
      responses:
        '200':
          description: Page of notifications.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/NotificationPage'
        '401':
          $ref: '#/components/responses/Unauthorized'
  /notifications/unread_count:
    get:
      tags: [notifications]
      summary: Unread badge counter
      operationId: getUnreadCount
      responses:
        '200':
          description: Count of unread notifications.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/UnreadCount'
        '401':
          $ref: '#/components/responses/Unauthorized'
  /notifications/read:
    post:
      tags: [notifications]
      summary: Mark all notifications up to an ID as read
      operationId: markNotificationsRead
      parameters:
        - $ref: '#/components/parameters/IdempotencyKey'
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/MarkReadRequest'
      responses:
        '204':
          description: Marked.
        '401':
          $ref: '#/components/responses/Unauthorized'
  /media:
    post:
      tags: [media]
      summary: Allocate media ID and pre-signed upload URL (§2.5)
      operationId: createMedia
      parameters:
        - $ref: '#/components/parameters/IdempotencyKey'
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/CreateMediaRequest'
      responses:
        '201':
          description: Upload ticket; client PUTs directly to storage.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/MediaUploadTicket'
        '400':
          $ref: '#/components/responses/BadRequest'
        '401':
          $ref: '#/components/responses/Unauthorized'
        '429':
          $ref: '#/components/responses/TooManyRequests'
  /media/{id}:
    get:
      tags: [media]
      summary: Media processing status
      operationId: getMedia
      parameters:
        - $ref: '#/components/parameters/MediaId'
      responses:
        '200':
          description: Status; poll until ready before attaching to a tweet.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Media'
        '401':
          $ref: '#/components/responses/Unauthorized'
        '404':
          $ref: '#/components/responses/NotFound'
  /media/{id}/complete:
    post:
      tags: [media]
      summary: Confirm upload — verifies object exists, emits MediaUploaded via outbox
      operationId: completeMedia
      parameters:
        - $ref: '#/components/parameters/MediaId'
      responses:
        '200':
          description: Accepted for processing (status becomes uploaded).
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Media'
        '401':
          $ref: '#/components/responses/Unauthorized'
        '404':
          $ref: '#/components/responses/NotFound'
        '409':
          description: Object not found in storage (client never PUT it).
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Error'
```

- [ ] **Step 3: Lint the complete spec**

Run: `make lint-spec`
Expected: exit 0.

- [ ] **Step 4: Commit**

```bash
git add api/openapi.yaml
git commit -m "feat(api): timelines, notifications, media endpoints - contract complete"
```

---

### Task 5: Codegen — server stubs + Go client

**Files:**
- Create: `api/server.cfg.yaml`, `api/client.cfg.yaml`
- Create (generated): `internal/httpapi/api.gen.go`, `pkg/apiclient/client.gen.go`
- Modify: `Makefile` (add `generate`)

- [ ] **Step 1: Create `api/server.cfg.yaml`**

```yaml
package: httpapi
generate:
  std-http-server: true
  models: true
output: internal/httpapi/api.gen.go
```

- [ ] **Step 2: Create `api/client.cfg.yaml`**

```yaml
package: apiclient
generate:
  client: true
  models: true
output: pkg/apiclient/client.gen.go
```

- [ ] **Step 3: Add `generate` to `Makefile`** (and to `.PHONY`)

```makefile
OAPI_CODEGEN = go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.4.1
BUF = go run github.com/bufbuild/buf/cmd/buf@v1.47.2

generate:
	$(OAPI_CODEGEN) -config api/server.cfg.yaml api/openapi.yaml
	$(OAPI_CODEGEN) -config api/client.cfg.yaml api/openapi.yaml
	$(BUF) generate
```

(`buf generate` no-ops until Task 6 adds `buf.gen.yaml`; if it errors on the missing file, add the buf line in Task 6 instead.)

- [ ] **Step 4: Generate and add the runtime dependency**

```bash
mkdir -p internal/httpapi pkg/apiclient
go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.4.1 -config api/server.cfg.yaml api/openapi.yaml
go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.4.1 -config api/client.cfg.yaml api/openapi.yaml
go get github.com/oapi-codegen/runtime
go mod tidy
```

Expected: both `.gen.go` files exist.

- [ ] **Step 5: Verify generated code compiles**

Run: `go build ./...`
Expected: exit 0. (`internal/httpapi.ServerInterface` now exists; Phase 1 tasks implement it. `pkg/apiclient.ClientWithResponses` is what seed/k6 will use.)

- [ ] **Step 6: Commit**

```bash
git add api Makefile internal/httpapi pkg/apiclient go.mod go.sum
git commit -m "feat(api): generated server stubs and go client from openapi"
```

---

### Task 6: Protobuf event contracts + buf + generated Go

**Files:**
- Create: `buf.yaml`, `buf.gen.yaml`
- Create: `proto/events/common/v1/envelope.proto`, `proto/events/{tweets,engagements,follows,media}/v1/events.proto`
- Create (generated): `gen/events/**`
- Test: `gen/roundtrip_test.go`

- [ ] **Step 1: Create `buf.yaml`**

```yaml
version: v2
modules:
  - path: proto
lint:
  use:
    - STANDARD
breaking:
  use:
    - FILE
```

- [ ] **Step 2: Create `buf.gen.yaml`**

```yaml
version: v2
inputs:
  - directory: proto
plugins:
  - remote: buf.build/protocolbuffers/go
    out: gen
    opt: paths=source_relative
```

- [ ] **Step 3: Create `proto/events/common/v1/envelope.proto`**

```protobuf
syntax = "proto3";

package yaxter.events.common.v1;

import "google/protobuf/timestamp.proto";

option go_package = "github.com/fonvacano/yaxter/gen/events/common/v1;commonv1";

// Envelope is carried by every event. Consumers dedupe on event_id
// (at-least-once delivery, ARCHITECTURE.md §2.4); traceparent propagates
// the W3C trace context across the outbox -> Kafka hop.
message Envelope {
  int64 event_id = 1;
  google.protobuf.Timestamp occurred_at = 2;
  string traceparent = 3;
  string producer = 4;
}
```

- [ ] **Step 4: Create `proto/events/tweets/v1/events.proto`** (topic `tweets.v1`, key `author_id`)

```protobuf
syntax = "proto3";

package yaxter.events.tweets.v1;

import "events/common/v1/envelope.proto";

option go_package = "github.com/fonvacano/yaxter/gen/events/tweets/v1;tweetsv1";

message TweetCreated {
  int64 tweet_id = 1;
  int64 author_id = 2;
  string text = 3;
  int64 retweet_of_id = 4; // 0 = not a retweet
  repeated int64 media_ids = 5;
  // Snapshot at write time; fan-out compares it to CELEBRITY_THRESHOLD
  // without an extra lookup.
  int32 author_followers_count = 6;
}

message TweetDeleted {
  int64 tweet_id = 1;
  int64 author_id = 2;
}

message TweetEvent {
  yaxter.events.common.v1.Envelope envelope = 1;
  oneof payload {
    TweetCreated created = 2;
    TweetDeleted deleted = 3;
  }
}
```

- [ ] **Step 5: Create `proto/events/engagements/v1/events.proto`** (topic `engagements.v1`, key `tweet_id`)

```protobuf
syntax = "proto3";

package yaxter.events.engagements.v1;

import "events/common/v1/envelope.proto";

option go_package = "github.com/fonvacano/yaxter/gen/events/engagements/v1;engagementsv1";

message TweetLiked {
  int64 tweet_id = 1;
  int64 user_id = 2;
  int64 author_id = 3; // notification recipient
}

message TweetUnliked {
  int64 tweet_id = 1;
  int64 user_id = 2;
  int64 author_id = 3;
}

message TweetRetweeted {
  int64 tweet_id = 1;   // the original tweet (partition key)
  int64 retweet_id = 2; // the new tweet row with retweet_of_id set
  int64 user_id = 3;
  int64 author_id = 4;
}

message TweetUnretweeted {
  int64 tweet_id = 1;
  int64 retweet_id = 2;
  int64 user_id = 3;
  int64 author_id = 4;
}

message EngagementEvent {
  yaxter.events.common.v1.Envelope envelope = 1;
  oneof payload {
    TweetLiked liked = 2;
    TweetUnliked unliked = 3;
    TweetRetweeted retweeted = 4;
    TweetUnretweeted unretweeted = 5;
  }
}
```

- [ ] **Step 6: Create `proto/events/follows/v1/events.proto`** (topic `follows.v1`, key `followee_id`)

```protobuf
syntax = "proto3";

package yaxter.events.follows.v1;

import "events/common/v1/envelope.proto";

option go_package = "github.com/fonvacano/yaxter/gen/events/follows/v1;followsv1";

message FollowChanged {
  int64 follower_id = 1;
  int64 followee_id = 2;
  bool following = 3; // true = follow, false = unfollow
}

message FollowEvent {
  yaxter.events.common.v1.Envelope envelope = 1;
  oneof payload {
    FollowChanged follow_changed = 2;
  }
}
```

- [ ] **Step 7: Create `proto/events/media/v1/events.proto`** (topic `media.v1`, key `media_id`)

```protobuf
syntax = "proto3";

package yaxter.events.media.v1;

import "events/common/v1/envelope.proto";

option go_package = "github.com/fonvacano/yaxter/gen/events/media/v1;mediav1";

message MediaUploaded {
  int64 media_id = 1;
  int64 owner_id = 2;
  string content_type = 3;
  int64 size_bytes = 4;
}

message MediaEvent {
  yaxter.events.common.v1.Envelope envelope = 1;
  oneof payload {
    MediaUploaded uploaded = 2;
  }
}
```

- [ ] **Step 8: Lint and generate**

```bash
go run github.com/bufbuild/buf/cmd/buf@v1.47.2 lint
go run github.com/bufbuild/buf/cmd/buf@v1.47.2 generate
go get google.golang.org/protobuf
go mod tidy
```

Expected: lint exit 0; `gen/events/{common,tweets,engagements,follows,media}/v1/*.pb.go` exist.

- [ ] **Step 9: Write the roundtrip test** — `gen/roundtrip_test.go`

```go
package gen_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/fonvacano/yaxter/gen/events/common/v1"
	tweetsv1 "github.com/fonvacano/yaxter/gen/events/tweets/v1"
)

func TestTweetEventRoundtrip(t *testing.T) {
	in := &tweetsv1.TweetEvent{
		Envelope: &commonv1.Envelope{
			EventId:     123456789,
			OccurredAt:  timestamppb.Now(),
			Traceparent: "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01",
			Producer:    "api@test",
		},
		Payload: &tweetsv1.TweetEvent_Created{Created: &tweetsv1.TweetCreated{
			TweetId:               1,
			AuthorId:              2,
			Text:                  "hello",
			MediaIds:              []int64{10, 11},
			AuthorFollowersCount:  42,
		}},
	}
	raw, err := proto.Marshal(in)
	require.NoError(t, err)

	var out tweetsv1.TweetEvent
	require.NoError(t, proto.Unmarshal(raw, &out))
	require.True(t, proto.Equal(in, &out))
	require.Equal(t, int64(123456789), out.GetEnvelope().GetEventId())
	require.Equal(t, "hello", out.GetCreated().GetText())
}
```

- [ ] **Step 10: Run the test**

Run: `go test ./gen/ -v`
Expected: PASS.

- [ ] **Step 11: Commit**

```bash
git add buf.yaml buf.gen.yaml proto gen go.mod go.sum
git commit -m "feat(events): protobuf contracts for all four topics with envelope"
```

---

### Task 7: `docs/events.md` — topic/key/ordering contract

**Files:**
- Create: `docs/events.md`

- [ ] **Step 1: Write the contract doc**

```markdown
# Event Contracts

Source of truth: `proto/events/**`. No schema registry in the demo — the proto
package *is* the registry, enforced by `buf lint` + `buf breaking` in CI
(ARCHITECTURE.md §2.4).

## Topics

| Topic | Key (→ partition) | Wrapper message | Producers → Consumers | Demo / Prod partitions |
|---|---|---|---|---|
| `tweets.v1` | `author_id` | `yaxter.events.tweets.v1.TweetEvent` | relay → fanout (future: search, ML) | 3 / 64 |
| `engagements.v1` | `tweet_id` | `yaxter.events.engagements.v1.EngagementEvent` | relay → counters, notifications | 3 / 64 |
| `follows.v1` | `followee_id` | `yaxter.events.follows.v1.FollowEvent` | relay → notifications, graph maintenance | 3 / 32 |
| `media.v1` | `media_id` | `yaxter.events.media.v1.MediaEvent` | relay → media worker | 1 / 8 |

Keys are encoded as **decimal strings** in the Kafka record key.

## Rules (binding on every producer and consumer)

1. **At-least-once + idempotent consumers.** The outbox relay may re-publish
   after a crash. Consumers MUST dedupe on `envelope.event_id`.
2. **Ordering is per-key only.** Events with the same key arrive in order
   (relay publishes in snowflake order; key keeps partition affinity).
   Nothing is guaranteed across keys.
3. **No historical key→partition affinity.** Partition counts grow online and
   keys re-hash. Consumers MUST NOT persist any assumption that key K lives in
   partition P. (This makes promotion step 5 of §4 safe.)
4. **Additive-only evolution.** New fields get new tag numbers; never reuse or
   renumber. Removed fields are `reserved`. A breaking change = a new topic
   with the next `vN` suffix and a new proto package version.
5. **Trace context** travels in `envelope.traceparent` AND in the Kafka
   `traceparent` message header (so generic tooling sees it too).
6. **Consumer groups** are named `yaxter.<role>` (see `pkg/kafkax.GroupID`).
```

- [ ] **Step 2: Commit**

```bash
git add docs/events.md
git commit -m "docs(events): topic, key, ordering, and evolution contract"
```

---

### Task 8: DB migrations for the full §2.2 schema

**Files:**
- Create: `migrations/000001_extensions.{up,down}.sql` … `migrations/000007_outbox_node_ids.{up,down}.sql`
- Test: `migrations/migrations_test.go` (integration, skipped under `-short`)

- [ ] **Step 1: Write the failing test** — `migrations/migrations_test.go`

```go
package migrations_test

import (
	"context"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	pgxkit "github.com/fonvacano/yaxter/pkg/pgx"
)

var allTables = []string{
	"users", "identities", "global_identities",
	"follows", "followers",
	"tweets", "likes",
	"notifications",
	"refresh_tokens", "idempotency",
	"outbox", "node_ids",
}

func TestMigrationsUpDownUp(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires Docker")
	}
	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("yaxter"),
		tcpostgres.WithUsername("yaxter"),
		tcpostgres.WithPassword("yaxter"),
		tcpostgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	m, err := migrate.New("file://.", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { m.Close() })

	require.NoError(t, m.Up())

	pool, err := pgxkit.NewPool(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	for _, tbl := range allTables {
		var reg *string
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT to_regclass($1)::text`, tbl).Scan(&reg))
		require.NotNil(t, reg, "table %s must exist after up", tbl)
	}

	require.NoError(t, m.Down())
	var reg *string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT to_regclass('users')::text`).Scan(&reg))
	require.Nil(t, reg, "users must be gone after down")

	require.NoError(t, m.Up(), "re-up after down must be clean")
}
```

- [ ] **Step 2: Add the migrate dependency and run the test to verify it fails**

```bash
go get github.com/golang-migrate/migrate/v4
go test ./migrations/ -run TestMigrations -v
```

Expected: FAIL — no migration files found (or first `m.Up()` errors).

- [ ] **Step 3: Write the migration files**

`migrations/000001_extensions.up.sql`:

```sql
CREATE EXTENSION IF NOT EXISTS citext;
```

`migrations/000001_extensions.down.sql`:

```sql
DROP EXTENSION IF EXISTS citext;
```

`migrations/000002_users_identities.up.sql`:

```sql
-- Sharded by user_id (§2.2). UNIQUE on username/email is per-physical-shard;
-- global uniqueness is enforced via the global_* lookup tables below, which
-- the demo colocates on the single cluster.
CREATE TABLE users (
    id              BIGINT PRIMARY KEY,
    username        CITEXT NOT NULL UNIQUE,
    email           CITEXT NOT NULL UNIQUE,
    pass_hash       TEXT,                         -- NULL for OAuth-only accounts
    bio             TEXT   NOT NULL DEFAULT '',
    avatar_key      TEXT,
    followers_count INT    NOT NULL DEFAULT 0,
    following_count INT    NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE identities (
    user_id          BIGINT NOT NULL REFERENCES users (id),
    provider         TEXT   NOT NULL,
    provider_user_id TEXT   NOT NULL,
    email            CITEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, provider),
    UNIQUE (provider, provider_user_id)
);

-- Global lookup: (provider, provider_user_id) -> user, for OAuth login
-- before the user's shard is known (§2.2 "global lookup tables").
CREATE TABLE global_identities (
    provider         TEXT   NOT NULL,
    provider_user_id TEXT   NOT NULL,
    user_id          BIGINT NOT NULL,
    PRIMARY KEY (provider, provider_user_id)
);
```

`migrations/000002_users_identities.down.sql`:

```sql
DROP TABLE global_identities;
DROP TABLE identities;
DROP TABLE users;
```

`migrations/000003_graph.up.sql`:

```sql
-- follows sharded by follower_id ("who do I follow"); followers is the
-- duplicated reverse edge sharded by followee_id ("who follows X"), kept in
-- sync via FollowChanged (§2.2). No FKs: edges cross shards.
CREATE TABLE follows (
    follower_id BIGINT NOT NULL,
    followee_id BIGINT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (follower_id, followee_id)
);

CREATE TABLE followers (
    followee_id BIGINT NOT NULL,
    follower_id BIGINT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (followee_id, follower_id)
);
```

`migrations/000003_graph.down.sql`:

```sql
DROP TABLE followers;
DROP TABLE follows;
```

`migrations/000004_tweets_likes.up.sql`:

```sql
-- tweets sharded by author_id: profile timeline is single-shard. The
-- (author_id, id DESC) index serves cursor pagination index-only (§2.6).
CREATE TABLE tweets (
    id             BIGINT PRIMARY KEY,            -- snowflake
    author_id      BIGINT NOT NULL,
    text           VARCHAR(280) NOT NULL,
    retweet_of_id  BIGINT,
    media          JSONB,
    likes_count    INT NOT NULL DEFAULT 0,        -- denormalized, eventual (§2.7)
    retweets_count INT NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX tweets_author_id_id_idx ON tweets (author_id, id DESC);

CREATE TABLE likes (
    user_id    BIGINT NOT NULL,
    tweet_id   BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, tweet_id)               -- idempotent ON CONFLICT DO NOTHING
);
CREATE INDEX likes_tweet_id_idx ON likes (tweet_id); -- nightly reconcile scans
```

`migrations/000004_tweets_likes.down.sql`:

```sql
DROP TABLE likes;
DROP TABLE tweets;
```

`migrations/000005_notifications.up.sql`:

```sql
CREATE TABLE notifications (
    id         BIGINT PRIMARY KEY,                -- snowflake
    user_id    BIGINT NOT NULL,
    kind       TEXT   NOT NULL,
    actor_id   BIGINT NOT NULL,
    subject_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    read_at    TIMESTAMPTZ
);
CREATE INDEX notifications_user_id_id_idx ON notifications (user_id, id DESC);
CREATE INDEX notifications_unread_idx ON notifications (user_id) WHERE read_at IS NULL;
```

`migrations/000005_notifications.down.sql`:

```sql
DROP TABLE notifications;
```

`migrations/000006_auth.up.sql`:

```sql
-- Rotating refresh tokens with reuse-detection: reuse of a rotated token
-- revokes the whole family_id (§2.8).
CREATE TABLE refresh_tokens (
    id         BIGINT PRIMARY KEY,                -- snowflake
    user_id    BIGINT NOT NULL,
    family_id  BIGINT NOT NULL,
    token_hash TEXT   NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ
);
CREATE INDEX refresh_tokens_user_id_idx ON refresh_tokens (user_id);

-- Durable tier of the idempotency store (Redis is the hot tier, §2.3).
CREATE TABLE idempotency (
    key           UUID PRIMARY KEY,
    user_id       BIGINT NOT NULL,
    response_hash TEXT   NOT NULL,
    expires_at    TIMESTAMPTZ NOT NULL
);
```

`migrations/000006_auth.down.sql`:

```sql
DROP TABLE idempotency;
DROP TABLE refresh_tokens;
```

`migrations/000007_outbox_node_ids.up.sql`:

```sql
-- Outbox rows are written in the SAME tx as the domain row and deleted soon
-- after publish; fillfactor + aggressive autovacuum keep churn cheap (§2.4).
CREATE TABLE outbox (
    id           BIGINT PRIMARY KEY,              -- snowflake = publish order
    topic        TEXT  NOT NULL,
    key          TEXT  NOT NULL,
    payload      BYTEA NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
) WITH (fillfactor = 70);
ALTER TABLE outbox SET (
    autovacuum_vacuum_scale_factor = 0.01,
    autovacuum_vacuum_cost_delay = 0
);
CREATE INDEX outbox_unpublished_idx ON outbox (id) WHERE published_at IS NULL;

-- Snowflake worker-ID leases (§2.6). Must stay compatible with the
-- CREATE TABLE IF NOT EXISTS in pkg/snowflake/lease.go (T0.1).
CREATE TABLE IF NOT EXISTS node_ids (
    node_id      INT PRIMARY KEY,
    leased_by    TEXT NOT NULL,
    heartbeat_at TIMESTAMPTZ NOT NULL
);
```

`migrations/000007_outbox_node_ids.down.sql`:

```sql
DROP TABLE IF EXISTS node_ids;
DROP TABLE outbox;
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./migrations/ -run TestMigrations -v` (Docker running)
Expected: PASS — up, all 12 tables present, down clean, re-up clean.

- [ ] **Step 5: Commit**

```bash
git add migrations go.mod go.sum
git commit -m "feat(db): full schema migrations with outbox tuning and global lookups"
```

---

### Task 9: `pkg/sharding` — logical-shard hash + shard map

The hash function is a **permanent contract**: `FNV-1a(64) over the big-endian 8 bytes of the key, mod 256`. Go's `hash/fnv` is a fixed, standardized algorithm, so stability is guaranteed by the stdlib. Plain `key % 256` is forbidden — snowflake low bits are the sequence (usually 0) and would funnel everything into a few shards.

**Files:**
- Create: `pkg/sharding/shard.go`, `pkg/sharding/config.go`
- Test: `pkg/sharding/shard_test.go`

- [ ] **Step 1: Add dependency**

```bash
go get gopkg.in/yaml.v3
```

- [ ] **Step 2: Write the failing test** — `pkg/sharding/shard_test.go`

```go
package sharding

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLogicalShardDeterministicAndInRange(t *testing.T) {
	for _, key := range []int64{0, 1, 42, 1<<40 + 12345, -7} {
		s1 := LogicalShard(key)
		s2 := LogicalShard(key)
		require.Equal(t, s1, s2)
		require.GreaterOrEqual(t, s1, 0)
		require.Less(t, s1, NumLogicalShards)
	}
}

// Snowflake-shaped keys (low 12 bits = sequence, usually 0) must still
// spread across shards — this is the test that forbids `key % 256`.
func TestLogicalShardDistributionOnSnowflakes(t *testing.T) {
	shards := make(map[int]bool)
	for i := int64(0); i < 10000; i++ {
		key := (1700000000000+i)<<22 | 5<<12 // seq always 0, node always 5
		shards[LogicalShard(key)] = true
	}
	require.GreaterOrEqual(t, len(shards), 200,
		"10k snowflake keys must hit most of the 256 shards")
}

func TestNewMapValidatesCoverage(t *testing.T) {
	// gap
	_, err := NewMap(Config{Physical: []Physical{
		{Name: "a", DSN: "x", Logicals: "0-100"},
	}})
	require.ErrorContains(t, err, "unassigned")

	// overlap
	_, err = NewMap(Config{Physical: []Physical{
		{Name: "a", DSN: "x", Logicals: "0-200"},
		{Name: "b", DSN: "y", Logicals: "100-255"},
	}})
	require.ErrorContains(t, err, "twice")

	// exact cover, split ranges
	m, err := NewMap(Config{Physical: []Physical{
		{Name: "a", DSN: "x", Logicals: "0-63,128-191"},
		{Name: "b", DSN: "y", Logicals: "64-127,192-255"},
	}})
	require.NoError(t, err)
	require.NotNil(t, m)
}

func TestPhysicalForConsistentWithLogicalShard(t *testing.T) {
	m, err := NewMap(Config{Physical: []Physical{
		{Name: "a", DSN: "x", Logicals: "0-127"},
		{Name: "b", DSN: "y", Logicals: "128-255"},
	}})
	require.NoError(t, err)

	for _, key := range []int64{1, 999, 123456789} {
		want := "a"
		if LogicalShard(key) >= 128 {
			want = "b"
		}
		require.Equal(t, want, m.PhysicalFor(key).Name)
	}
}

func TestParseConfigYAML(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
physical:
  - name: demo
    dsn: postgres://yaxter:yaxter@localhost:5432/yaxter
    logicals: "0-255"
`))
	require.NoError(t, err)
	m, err := NewMap(cfg)
	require.NoError(t, err)
	require.Equal(t, "demo", m.PhysicalFor(42).Name)
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./pkg/sharding/ -v`
Expected: FAIL — `undefined: LogicalShard` etc.

- [ ] **Step 4: Write `pkg/sharding/shard.go`**

```go
// Package sharding implements the 256-logical-shard routing from
// ARCHITECTURE.md §2.2. NumLogicalShards and LogicalShard are permanent
// contracts: production remaps logical shards to new physical clusters by
// editing the shard map — never by changing the hash.
package sharding

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
)

const NumLogicalShards = 256

// LogicalShard maps a shard key (user_id, author_id, ...) to its logical
// shard: FNV-1a(64) over the key's big-endian bytes, mod 256.
func LogicalShard(key int64) int {
	h := fnv.New64a()
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(key))
	_, _ = h.Write(b[:])
	return int(h.Sum64() % NumLogicalShards)
}

// Map resolves logical shards to physical clusters.
type Map struct {
	physicals []Physical
	byLogical [NumLogicalShards]int // logical -> index into physicals
}

func NewMap(cfg Config) (*Map, error) {
	m := &Map{physicals: cfg.Physical}
	for i := range m.byLogical {
		m.byLogical[i] = -1
	}
	for pi, p := range cfg.Physical {
		logicals, err := parseRanges(p.Logicals)
		if err != nil {
			return nil, fmt.Errorf("sharding: physical %q: %w", p.Name, err)
		}
		for _, l := range logicals {
			if m.byLogical[l] != -1 {
				return nil, fmt.Errorf("sharding: logical shard %d assigned twice", l)
			}
			m.byLogical[l] = pi
		}
	}
	for l, pi := range m.byLogical {
		if pi == -1 {
			return nil, fmt.Errorf("sharding: logical shard %d unassigned", l)
		}
	}
	return m, nil
}

// PhysicalFor returns the physical cluster owning key's logical shard.
func (m *Map) PhysicalFor(key int64) Physical {
	return m.physicals[m.byLogical[LogicalShard(key)]]
}

// Physicals returns all configured physical clusters.
func (m *Map) Physicals() []Physical { return m.physicals }
```

- [ ] **Step 5: Write `pkg/sharding/config.go`**

```go
package sharding

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Physical struct {
	Name     string `yaml:"name"`
	DSN      string `yaml:"dsn"`
	Logicals string `yaml:"logicals"` // e.g. "0-255" or "0-63,128-191" or "5"
}

type Config struct {
	Physical []Physical `yaml:"physical"`
}

func ParseConfig(data []byte) (Config, error) {
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("sharding: parse config: %w", err)
	}
	return c, nil
}

// parseRanges expands "0-63,128-191" into the listed logical shard numbers.
func parseRanges(s string) ([]int, error) {
	var out []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		lo, hi := part, part
		if i := strings.IndexByte(part, '-'); i >= 0 {
			lo, hi = part[:i], part[i+1:]
		}
		l, err := strconv.Atoi(lo)
		if err != nil {
			return nil, fmt.Errorf("bad range %q: %w", part, err)
		}
		h, err := strconv.Atoi(hi)
		if err != nil {
			return nil, fmt.Errorf("bad range %q: %w", part, err)
		}
		if l > h || l < 0 || h >= NumLogicalShards {
			return nil, fmt.Errorf("range %q out of bounds [0,%d]", part, NumLogicalShards-1)
		}
		for n := l; n <= h; n++ {
			out = append(out, n)
		}
	}
	return out, nil
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./pkg/sharding/ -race -v`
Expected: PASS (5 tests).

- [ ] **Step 7: Commit**

```bash
git add pkg/sharding go.mod go.sum
git commit -m "feat(sharding): fnv logical-shard hash and validated shard map"
```

---

### Task 10: `pkg/sharding` — router with pools + per-shard batched helper

**Files:**
- Create: `pkg/sharding/router.go`, `configs/shardmap.yaml`
- Test: `pkg/sharding/router_test.go`

- [ ] **Step 1: Write the failing test** — `pkg/sharding/router_test.go`

```go
package sharding

import (
	"context"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// ForEachShard is pure Map logic — unit-testable with a 2-entry fake map
// (the T0.4 DoD case). In the demo the grouping degenerates to one batch,
// but the code path is exercised here with two.
func TestForEachShardGroupsKeys(t *testing.T) {
	m, err := NewMap(Config{Physical: []Physical{
		{Name: "a", DSN: "x", Logicals: "0-127"},
		{Name: "b", DSN: "y", Logicals: "128-255"},
	}})
	require.NoError(t, err)

	keys := make([]int64, 0, 1000)
	for i := int64(1); i <= 1000; i++ {
		keys = append(keys, i*7919) // arbitrary spread
	}

	var got []int64
	seen := map[string][]int64{}
	err = m.ForEachShard(keys, func(p Physical, shardKeys []int64) error {
		seen[p.Name] = append(seen[p.Name], shardKeys...)
		got = append(got, shardKeys...)
		for _, k := range shardKeys {
			want := "a"
			if LogicalShard(k) >= 128 {
				want = "b"
			}
			require.Equal(t, want, p.Name, "key %d routed to wrong shard", k)
		}
		return nil
	})
	require.NoError(t, err)
	require.Len(t, seen, 2, "1000 spread keys must hit both physicals")

	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	require.Equal(t, keys, got, "every key visited exactly once")
}

func TestRouterPoolsAgainstRealPG(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires Docker")
	}
	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("yaxter"),
		tcpostgres.WithUsername("yaxter"),
		tcpostgres.WithPassword("yaxter"),
		tcpostgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	m, err := NewMap(Config{Physical: []Physical{
		{Name: "demo", DSN: dsn, Logicals: "0-255"},
	}})
	require.NoError(t, err)

	r, err := NewRouter(ctx, m)
	require.NoError(t, err)
	t.Cleanup(r.Close)

	var one int
	require.NoError(t, r.Pool(42).QueryRow(ctx, "SELECT 1").Scan(&one))
	require.Equal(t, 1, one)

	calls := 0
	err = r.ForEachShard([]int64{1, 2, 3}, func(pool Pool, keys []int64) error {
		calls++
		require.Len(t, keys, 3)
		return pool.QueryRow(ctx, "SELECT 1").Scan(&one)
	})
	require.NoError(t, err)
	require.Equal(t, 1, calls, "single physical => single batch")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/sharding/ -run 'TestForEachShard|TestRouter' -v`
Expected: FAIL — `undefined: (*Map).ForEachShard`, `undefined: NewRouter`.

- [ ] **Step 3: Write `pkg/sharding/router.go`**

```go
package sharding

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ForEachShard groups keys by owning physical shard (preserving input order
// within a group) and calls fn once per physical. Cross-shard reads batch
// per physical cluster (§2.2); demo cardinality makes this one call.
func (m *Map) ForEachShard(keys []int64, fn func(p Physical, keys []int64) error) error {
	groups := make(map[int][]int64)
	var order []int
	for _, k := range keys {
		pi := m.byLogical[LogicalShard(k)]
		if _, ok := groups[pi]; !ok {
			order = append(order, pi)
		}
		groups[pi] = append(groups[pi], k)
	}
	for _, pi := range order {
		if err := fn(m.physicals[pi], groups[pi]); err != nil {
			return err
		}
	}
	return nil
}

// Pool is the query surface repositories use — satisfied by *pgxpool.Pool
// and by pgx transactions in tests.
type Pool interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (interface{ RowsAffected() int64 }, error)
}

// Router owns one pgx pool per physical cluster.
type Router struct {
	m     *Map
	pools map[string]*pgxpool.Pool
}

func NewRouter(ctx context.Context, m *Map) (*Router, error) {
	r := &Router{m: m, pools: make(map[string]*pgxpool.Pool, len(m.physicals))}
	for _, p := range m.physicals {
		pool, err := pgxpool.New(ctx, p.DSN)
		if err != nil {
			r.Close()
			return nil, fmt.Errorf("sharding: dial %q: %w", p.Name, err)
		}
		if err := pool.Ping(ctx); err != nil {
			pool.Close()
			r.Close()
			return nil, fmt.Errorf("sharding: ping %q: %w", p.Name, err)
		}
		r.pools[p.Name] = pool
	}
	return r, nil
}

// Pool returns the pool owning key's shard.
func (r *Router) Pool(key int64) *pgxpool.Pool {
	return r.pools[r.m.PhysicalFor(key).Name]
}

// ForEachShard runs fn once per physical shard holding any of keys.
func (r *Router) ForEachShard(keys []int64, fn func(pool *pgxpool.Pool, keys []int64) error) error {
	return r.m.ForEachShard(keys, func(p Physical, ks []int64) error {
		return fn(r.pools[p.Name], ks)
	})
}

func (r *Router) Close() {
	for _, p := range r.pools {
		p.Close()
	}
}
```

**Implementation note:** the `Pool` interface in the test takes the router's `*pgxpool.Pool` directly — if the `Exec` anonymous-interface signature fights pgx's `pgconn.CommandTag`, drop the `Pool` interface entirely and use `*pgxpool.Pool` concretely in `ForEachShard` (as the test's second callback already does). Do not invent adapters; the simplest version that passes the test wins.

- [ ] **Step 4: Create `configs/shardmap.yaml`** (demo: all 256 → one cluster)

```yaml
# Demo shard map: 256 logical shards on one physical cluster.
# Production remaps groups of logicals to new clusters here — no code change.
physical:
  - name: demo
    dsn: postgres://yaxter:yaxter@localhost:5432/yaxter?sslmode=disable
    logicals: "0-255"
```

- [ ] **Step 5: Run tests**

Run: `go test ./pkg/sharding/ -race -v` (Docker running)
Expected: PASS — all Task 9 tests plus `TestForEachShardGroupsKeys` and `TestRouterPoolsAgainstRealPG`.

- [ ] **Step 6: Commit**

```bash
git add pkg/sharding configs
git commit -m "feat(sharding): pooled router and per-shard batched query helper"
```

---

### Task 11: `pkg/outbox` — same-transaction insert helper

**Files:**
- Create: `pkg/outbox/outbox.go`
- Test: `pkg/outbox/outbox_test.go` (integration, skipped under `-short`)

- [ ] **Step 1: Write the failing test** — `pkg/outbox/outbox_test.go`

```go
package outbox

import (
	"context"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	pgxkit "github.com/fonvacano/yaxter/pkg/pgx"
)

func setup(t *testing.T) (context.Context, *tcpostgres.PostgresContainer, string) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: requires Docker")
	}
	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("yaxter"),
		tcpostgres.WithUsername("yaxter"),
		tcpostgres.WithPassword("yaxter"),
		tcpostgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	m, err := migrate.New("file://../../migrations", dsn)
	require.NoError(t, err)
	require.NoError(t, m.Up())
	m.Close()
	return ctx, ctr, dsn
}

func TestInsertSharesTheCallersTransaction(t *testing.T) {
	ctx, _, dsn := setup(t)
	pool, err := pgxkit.NewPool(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	msg := Message{ID: 1001, Topic: "tweets.v1", Key: "2", Payload: []byte{0x1}}

	// Rollback: neither the domain row nor the outbox row survives.
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	_, err = tx.Exec(ctx,
		`INSERT INTO users (id, username, email) VALUES (2, 'alice', 'a@example.com')`)
	require.NoError(t, err)
	require.NoError(t, Insert(ctx, tx, msg))
	require.NoError(t, tx.Rollback(ctx))

	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM outbox`).Scan(&n))
	require.Zero(t, n)
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n))
	require.Zero(t, n)

	// Commit: both rows land atomically.
	tx, err = pool.Begin(ctx)
	require.NoError(t, err)
	_, err = tx.Exec(ctx,
		`INSERT INTO users (id, username, email) VALUES (2, 'alice', 'a@example.com')`)
	require.NoError(t, err)
	require.NoError(t, Insert(ctx, tx, msg))
	require.NoError(t, tx.Commit(ctx))

	var topic, key string
	var published *string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT topic, key, published_at::text FROM outbox WHERE id = 1001`,
	).Scan(&topic, &key, &published))
	require.Equal(t, "tweets.v1", topic)
	require.Equal(t, "2", key)
	require.Nil(t, published, "new rows are unpublished")
}

func TestInsertValidates(t *testing.T) {
	err := validate(Message{ID: 0, Topic: "t", Key: "k", Payload: []byte{1}})
	require.ErrorContains(t, err, "id")
	err = validate(Message{ID: 1, Topic: "", Key: "k", Payload: []byte{1}})
	require.ErrorContains(t, err, "topic")
	err = validate(Message{ID: 1, Topic: "t", Key: "", Payload: []byte{1}})
	require.ErrorContains(t, err, "key")
	require.NoError(t, validate(Message{ID: 1, Topic: "t", Key: "k", Payload: []byte{1}}))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/outbox/ -v`
Expected: FAIL — `undefined: Message`.

- [ ] **Step 3: Write `pkg/outbox/outbox.go`**

```go
// Package outbox implements the producer half of the transactional outbox
// (ARCHITECTURE.md §2.4): events are inserted in the SAME transaction as the
// domain write — atomicity is the database's. The relay worker (T1.0)
// publishes and deletes the rows; api never talks to Kafka.
package outbox

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Message is one event bound for Kafka. ID is a snowflake and defines
// publish order; Key becomes the Kafka record key (partition affinity).
type Message struct {
	ID      int64
	Topic   string
	Key     string
	Payload []byte // serialized protobuf event wrapper
}

func validate(m Message) error {
	switch {
	case m.ID == 0:
		return errors.New("outbox: message id required")
	case m.Topic == "":
		return errors.New("outbox: topic required")
	case m.Key == "":
		return errors.New("outbox: key required")
	}
	return nil
}

// Insert writes msg into the outbox using the caller's transaction.
// The caller MUST pass the same tx that performs the domain write.
func Insert(ctx context.Context, tx pgx.Tx, msg Message) error {
	if err := validate(msg); err != nil {
		return err
	}
	_, err := tx.Exec(ctx,
		`INSERT INTO outbox (id, topic, key, payload) VALUES ($1, $2, $3, $4)`,
		msg.ID, msg.Topic, msg.Key, msg.Payload)
	return err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/outbox/ -race -v` (Docker running)
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add pkg/outbox
git commit -m "feat(outbox): same-transaction insert helper"
```

---

### Task 12: docker-compose dev stack

Compose runs the *data plane* (same graph as production, §6); `api`/`worker` run on the host via `go run` during development and join the deployment story in T3.2.

**Files:**
- Create: `docker-compose.yaml`
- Modify: `Makefile` (add `up`, `down`)

- [ ] **Step 1: Create `docker-compose.yaml`**

```yaml
name: yaxter

services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: yaxter
      POSTGRES_PASSWORD: yaxter
      POSTGRES_DB: yaxter
    ports:
      - "5432:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U yaxter"]
      interval: 2s
      timeout: 3s
      retries: 30

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 2s
      timeout: 3s
      retries: 30

  kafka:
    image: apache/kafka:3.8.0
    ports:
      - "9092:9092"
    environment:
      KAFKA_NODE_ID: 1
      KAFKA_PROCESS_ROLES: broker,controller
      KAFKA_CONTROLLER_QUORUM_VOTERS: 1@kafka:9093
      KAFKA_LISTENERS: PLAINTEXT://:19092,CONTROLLER://:9093,EXTERNAL://:9092
      KAFKA_ADVERTISED_LISTENERS: PLAINTEXT://kafka:19092,EXTERNAL://localhost:9092
      KAFKA_LISTENER_SECURITY_PROTOCOL_MAP: PLAINTEXT:PLAINTEXT,CONTROLLER:PLAINTEXT,EXTERNAL:PLAINTEXT
      KAFKA_CONTROLLER_LISTENER_NAMES: CONTROLLER
      KAFKA_INTER_BROKER_LISTENER_NAME: PLAINTEXT
      KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR: 1
      KAFKA_AUTO_CREATE_TOPICS_ENABLE: "false"
    healthcheck:
      test: ["CMD-SHELL", "/opt/kafka/bin/kafka-broker-api-versions.sh --bootstrap-server localhost:19092 >/dev/null 2>&1"]
      interval: 5s
      timeout: 10s
      retries: 30

  minio:
    image: minio/minio:RELEASE.2024-11-07T00-52-20Z
    command: server /data --console-address ":9001"
    environment:
      MINIO_ROOT_USER: yaxter
      MINIO_ROOT_PASSWORD: yaxter123
    ports:
      - "9000:9000"
      - "9001:9001"
    healthcheck:
      test: ["CMD-SHELL", "curl -sf http://localhost:9000/minio/health/live"]
      interval: 2s
      timeout: 3s
      retries: 30

  jaeger:
    image: jaegertracing/all-in-one:1.62.0
    ports:
      - "16686:16686"   # UI
      - "4317:4317"     # OTLP gRPC

  mock-oauth:
    image: ghcr.io/navikt/mock-oauth2-server:2.1.10
    ports:
      - "9100:8080"

  # ---- one-shot init services (profile "init"; run via `make up`) ----

  migrate:
    image: migrate/migrate:v4.18.1
    profiles: ["init"]
    volumes:
      - ./migrations:/migrations:ro
    command:
      - "-path=/migrations"
      - "-database=postgres://yaxter:yaxter@postgres:5432/yaxter?sslmode=disable"
      - "up"
    depends_on:
      postgres:
        condition: service_healthy

  kafka-init:
    image: apache/kafka:3.8.0
    profiles: ["init"]
    depends_on:
      kafka:
        condition: service_healthy
    entrypoint: ["/bin/bash", "-c"]
    command:
      - |
        set -e
        for t in tweets.v1:3 engagements.v1:3 follows.v1:3 media.v1:1; do
          /opt/kafka/bin/kafka-topics.sh --bootstrap-server kafka:19092 \
            --create --if-not-exists \
            --topic "$${t%%:*}" --partitions "$${t##*:}" --replication-factor 1
        done
        /opt/kafka/bin/kafka-topics.sh --bootstrap-server kafka:19092 --list

  minio-init:
    image: minio/mc:RELEASE.2024-11-05T11-29-45Z
    profiles: ["init"]
    depends_on:
      minio:
        condition: service_healthy
    entrypoint: ["/bin/sh", "-c"]
    command:
      - |
        mc alias set local http://minio:9000 yaxter yaxter123 &&
        mc mb --ignore-existing local/media local/web
```

- [ ] **Step 2: Add `up` / `down` to `Makefile`** (and `.PHONY`)

```makefile
up:
	docker compose up -d --wait postgres redis kafka minio jaeger mock-oauth
	docker compose run --rm migrate
	docker compose run --rm kafka-init
	docker compose run --rm minio-init

down:
	docker compose down -v
```

- [ ] **Step 3: Verify the stack from scratch**

```bash
make down || true
make up
docker compose ps                                   # all services healthy
docker compose exec kafka /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server kafka:19092 --list             # 4 topics listed
docker compose exec postgres psql -U yaxter -c '\dt'  # 12 tables + schema_migrations
curl -sf http://localhost:9100/default/.well-known/openid-configuration | head -c 200
```

Expected: `tweets.v1 engagements.v1 follows.v1 media.v1` in the topic list; tables present; mock OIDC discovery document returned.

- [ ] **Step 4: Commit**

```bash
git add docker-compose.yaml Makefile
git commit -m "feat(dev): docker-compose stack with migrations, topics, buckets, mock oauth"
```

---

### Task 13: `cmd/seed` skeleton + CI contract gates

**Files:**
- Create: `cmd/seed/main.go`
- Modify: `Makefile` (add `seed`, `lint-proto`), `.github/workflows/ci.yaml` (add `contracts` job)

- [ ] **Step 1: Write `cmd/seed/main.go`**

```go
// Command seed populates the demo dataset. T0.5 ships the harness skeleton:
// it verifies the dev stack is reachable and migrated. The full dataset
// (1k users, Zipf follow graph, ~5 celebrities, 20k tweets) lands in T4.2.
package main

import (
	"context"
	"flag"
	"os"
	"time"

	"github.com/fonvacano/yaxter/pkg/config"
	logkit "github.com/fonvacano/yaxter/pkg/log"
	pgxkit "github.com/fonvacano/yaxter/pkg/pgx"
	"github.com/fonvacano/yaxter/pkg/redisx"
)

func main() {
	users := flag.Int("users", 1000, "users to create (dataset arrives with T4.2)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	logger := logkit.New(os.Stdout, cfg.LogLevel, "seed")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dsn := cfg.PostgresDSN
	if dsn == "" {
		dsn = "postgres://yaxter:yaxter@localhost:5432/yaxter?sslmode=disable"
	}
	pool, err := pgxkit.NewPool(ctx, dsn)
	if err != nil {
		logger.Fatal().Err(err).Msg("postgres unreachable - run `make up` first")
	}
	defer pool.Close()

	var version int
	var dirty bool
	if err := pool.QueryRow(ctx,
		`SELECT version, dirty FROM schema_migrations`).Scan(&version, &dirty); err != nil || dirty {
		logger.Fatal().Err(err).Bool("dirty", dirty).Msg("migrations not applied cleanly")
	}

	rdb := redisx.NewClient(cfg.RedisAddr)
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		logger.Fatal().Err(err).Msg("redis unreachable")
	}

	logger.Info().
		Int("schema_version", version).
		Int("users_requested", *users).
		Msg("seed harness ready - full dataset generation arrives with T4.2")
}
```

- [ ] **Step 2: Add `seed` and `lint-proto` to `Makefile`** (and `.PHONY`)

```makefile
seed:
	go run ./cmd/seed

lint-proto:
	$(BUF) lint
```

- [ ] **Step 3: Verify against the running stack**

Run: `make up && make seed`
Expected: final log line contains `"seed harness ready"` with `schema_version: 7`.

- [ ] **Step 4: Add the `contracts` job to `.github/workflows/ci.yaml`**

Append under `jobs:`:

```yaml
  contracts:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - uses: actions/setup-node@v4
        with:
          node-version: 22
      - name: Lint OpenAPI
        run: npx -y @stoplight/spectral-cli lint --fail-severity=error api/openapi.yaml
      - name: Lint protos
        run: go run github.com/bufbuild/buf/cmd/buf@v1.47.2 lint
      - name: Proto breaking-change gate
        if: github.event_name == 'pull_request'
        run: go run github.com/bufbuild/buf/cmd/buf@v1.47.2 breaking --against '.git#branch=origin/main'
      - name: Generated code is up to date
        run: |
          make generate
          git diff --exit-code
```

- [ ] **Step 5: Run the full local gate one last time**

```bash
make lint-spec
make lint-proto
make generate && git diff --exit-code
make test
make test-integration   # Docker required
make build
```

Expected: all exit 0.

- [ ] **Step 6: Commit**

```bash
git add cmd/seed Makefile .github
git commit -m "feat(dev): seed harness skeleton and CI contract gates"
```

---

## Phase 0 Definition-of-Done Check (run after Task 13)

| ARCHITECTURE.md DoD | Verified by |
|---|---|
| T0.2: spec lints (spectral); stubs compile; client exists for seed/k6 | Tasks 4–5; `pkg/apiclient` imported by `cmd/seed` successors |
| T0.3: buf lint + breaking pass in CI | Tasks 6, 13 (`contracts` job) |
| T0.4: migrations up/down clean on PG16; router unit-tested incl. 2-entry fake-map batching | Task 8 test; Task 10 `TestForEachShardGroupsKeys` |
| T0.5: `make up` healthy from scratch; topics exist; migrations auto-applied | Task 12 Step 3; Task 13 Step 3 |

## Out of Scope (deferred per ARCHITECTURE.md)

- Outbox **relay** worker (consume/publish/delete) — **T1.0** (this plan ships only the insert helper)
- Handler implementations of `httpapi.ServerInterface` — **T1.1–T2.3**
- Full seed dataset + k6 — **T4.2**
- Helm/Terraform/observability manifests — **T3.x**
