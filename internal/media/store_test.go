package media

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	tcminio "github.com/testcontainers/testcontainers-go/modules/minio"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: requires Docker")
	}
	ctx := context.Background()
	ctr, err := tcminio.Run(ctx, "minio/minio:RELEASE.2024-11-07T00-52-20Z")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })
	endpoint, err := ctr.ConnectionString(ctx)
	require.NoError(t, err)

	st, err := NewStore(ctx, StoreConfig{
		Endpoint:     "http://" + endpoint,
		Region:       "us-east-1",
		AccessKey:    ctr.Username, // minioadmin defaults from the module
		SecretKey:    ctr.Password,
		Bucket:       "media",
		UsePathStyle: true,
	})
	require.NoError(t, err)
	require.NoError(t, st.EnsureBucket(ctx))
	return st
}

func TestPresignedPutRoundtrip(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	url, err := st.PresignPut(ctx, uploadKey(42), "image/png")
	require.NoError(t, err)
	require.True(t, strings.Contains(url, "orig/42"))

	// The client-side PUT, exactly as the browser does it.
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader([]byte("pngbytes")))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "image/png")
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	size, err := st.Head(ctx, uploadKey(42))
	require.NoError(t, err)
	require.EqualValues(t, 8, size)

	raw, err := st.Get(ctx, uploadKey(42))
	require.NoError(t, err)
	require.Equal(t, []byte("pngbytes"), raw)

	require.NoError(t, st.Put(ctx, variantKey("feed", 42), []byte("webp"), "image/webp"))
}

func TestHeadMissingObject(t *testing.T) {
	st := testStore(t)
	_, err := st.Head(context.Background(), uploadKey(404))
	require.ErrorIs(t, err, ErrNoObject)
}
