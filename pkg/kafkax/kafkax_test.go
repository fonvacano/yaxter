package kafkax

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewClientRejectsEmptyBrokers(t *testing.T) {
	_, err := NewClient(nil)
	require.Error(t, err)
	_, err = NewClient([]string{})
	require.Error(t, err)
}

func TestNewClientBuilds(t *testing.T) {
	// franz-go connects lazily, so construction succeeds without a broker.
	c, err := NewClient([]string{"localhost:9092"})
	require.NoError(t, err)
	c.Close()
}

func TestGroupID(t *testing.T) {
	require.Equal(t, "yaxter.fanout", GroupID("fanout"))
}
