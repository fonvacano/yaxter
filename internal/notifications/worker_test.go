package notifications

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/fonvacano/yaxter/gen/yaxter/events/common/v1"
	engagementsv1 "github.com/fonvacano/yaxter/gen/yaxter/events/engagements/v1"
	followsv1 "github.com/fonvacano/yaxter/gen/yaxter/events/follows/v1"
	pgxkit "github.com/fonvacano/yaxter/pkg/pgx"
	"github.com/fonvacano/yaxter/pkg/snowflake"
)

func migratedPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
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
	_, _ = m.Close()
	pool, err := pgxkit.NewPool(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func newWorker(t *testing.T, pool *pgxpool.Pool) (*Worker, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	gen, err := snowflake.New(5)
	require.NoError(t, err)
	return NewWorker(pool, rdb, gen), rdb
}

func envelope(id int64) *commonv1.Envelope {
	return &commonv1.Envelope{EventId: id, OccurredAt: timestamppb.New(time.Unix(0, 0)), Producer: "api"}
}

func TestFollowEventCreatesNotificationForFollowee(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires Docker")
	}
	ctx := context.Background()
	pool := migratedPool(t, ctx)
	w, _ := newWorker(t, pool)

	ev := &followsv1.FollowEvent{Envelope: envelope(1000),
		Payload: &followsv1.FollowEvent_FollowChanged{FollowChanged: &followsv1.FollowChanged{
			FollowerId: 2, FolloweeId: 1, Following: true}}}
	require.NoError(t, w.handleFollow(ctx, ev))

	var userID, actorID int64
	var kind string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT user_id, kind, actor_id FROM notifications`).Scan(&userID, &kind, &actorID))
	require.EqualValues(t, 1, userID) // followee notified
	require.Equal(t, KindFollow, kind)
	require.EqualValues(t, 2, actorID)
}

func TestUnfollowProducesNoNotification(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires Docker")
	}
	ctx := context.Background()
	pool := migratedPool(t, ctx)
	w, _ := newWorker(t, pool)

	ev := &followsv1.FollowEvent{Envelope: envelope(1001),
		Payload: &followsv1.FollowEvent_FollowChanged{FollowChanged: &followsv1.FollowChanged{
			FollowerId: 2, FolloweeId: 1, Following: false}}}
	require.NoError(t, w.handleFollow(ctx, ev))

	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM notifications`).Scan(&n))
	require.Equal(t, 0, n)
}

func TestLikeNotifiesAuthorButNotSelf(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires Docker")
	}
	ctx := context.Background()
	pool := migratedPool(t, ctx)
	w, _ := newWorker(t, pool)

	// like by a different user → notification
	like := &engagementsv1.EngagementEvent{Envelope: envelope(1002),
		Payload: &engagementsv1.EngagementEvent_Liked{Liked: &engagementsv1.TweetLiked{
			TweetId: 99, UserId: 3, AuthorId: 1}}}
	require.NoError(t, w.handleEngagement(ctx, like))

	// self-like → suppressed
	selfLike := &engagementsv1.EngagementEvent{Envelope: envelope(1003),
		Payload: &engagementsv1.EngagementEvent_Liked{Liked: &engagementsv1.TweetLiked{
			TweetId: 99, UserId: 1, AuthorId: 1}}}
	require.NoError(t, w.handleEngagement(ctx, selfLike))

	var n int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM notifications WHERE kind = $1`, KindLike).Scan(&n))
	require.Equal(t, 1, n, "one like notification, self-like suppressed")
}

func TestEngagementReplayDeduped(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires Docker")
	}
	ctx := context.Background()
	pool := migratedPool(t, ctx)
	w, _ := newWorker(t, pool)

	ev := &engagementsv1.EngagementEvent{Envelope: envelope(1004),
		Payload: &engagementsv1.EngagementEvent_Retweeted{Retweeted: &engagementsv1.TweetRetweeted{
			TweetId: 99, RetweetId: 100, UserId: 3, AuthorId: 1}}}
	require.NoError(t, w.HandleEngagement(ctx, recordFor(ev))) // wrapper dedupes
	require.NoError(t, w.HandleEngagement(ctx, recordFor(ev)))

	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM notifications`).Scan(&n))
	require.Equal(t, 1, n, "replayed event must not double-insert")
}

func recordFor(ev *engagementsv1.EngagementEvent) *kgo.Record {
	raw, _ := proto.Marshal(ev)
	return &kgo.Record{Topic: "engagements.v1", Value: raw}
}
