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
