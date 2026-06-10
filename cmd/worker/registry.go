package main

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/fonvacano/yaxter/pkg/config"
)

type roleRunner func(ctx context.Context, logger zerolog.Logger, cfg config.Config)

// roleRunners is populated by init() in each role's file. Roles without a
// real implementation yet fall back to the placeholder heartbeat loop.
var roleRunners = map[string]roleRunner{}
