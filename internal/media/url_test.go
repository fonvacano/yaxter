package media

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// §2.5: the URL scheme is fixed forever — demo serves it from ALB→S3,
// production moves the same hostname to CDN. This test pins the contract.
func TestURLSchemeFixedForever(t *testing.T) {
	require.Equal(t,
		"https://media.example.com/feed/123.webp",
		URL("https://media.example.com", "feed", 123))
	require.Equal(t,
		"https://media.example.com/thumb/123.webp",
		URL("https://media.example.com", "thumb", 123))
	require.Equal(t,
		"https://media.example.com/orig/123.webp",
		URL("https://media.example.com", "orig", 123))
}

func TestObjectKeys(t *testing.T) {
	require.Equal(t, "orig/123", uploadKey(123), "raw upload key has no extension")
	require.Equal(t, "feed/123.webp", variantKey("feed", 123))
}
