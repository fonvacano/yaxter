package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveRolesDefaultsToAll(t *testing.T) {
	roles, err := resolveRoles(nil)
	require.NoError(t, err)
	require.Equal(t, []string{"relay", "fanout", "counters", "notifications", "media"}, roles)
}

func TestResolveRolesValidates(t *testing.T) {
	roles, err := resolveRoles([]string{"relay", "fanout"})
	require.NoError(t, err)
	require.Equal(t, []string{"relay", "fanout"}, roles)

	_, err = resolveRoles([]string{"relay", "bogus"})
	require.ErrorContains(t, err, "bogus")
}
