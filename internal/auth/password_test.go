package auth

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHashAndVerify(t *testing.T) {
	h, err := HashPassword("correct horse battery staple")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(h, "$argon2id$v=19$"))

	ok, err := VerifyPassword("correct horse battery staple", h)
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = VerifyPassword("wrong", h)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestHashesAreSalted(t *testing.T) {
	h1, err := HashPassword("same")
	require.NoError(t, err)
	h2, err := HashPassword("same")
	require.NoError(t, err)
	require.NotEqual(t, h1, h2)
}

func TestVerifyRejectsMalformed(t *testing.T) {
	_, err := VerifyPassword("x", "not-a-phc-string")
	require.Error(t, err)
}
