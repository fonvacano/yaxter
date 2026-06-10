package main

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/fonvacano/yaxter/pkg/config"
)

// roleRunner is a long-running function for a worker role.
type roleRunner func(ctx context.Context, logger zerolog.Logger, cfg config.Config)

// roleRunners is populated by each role's init() function.
var roleRunners = map[string]roleRunner{}
