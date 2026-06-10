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
