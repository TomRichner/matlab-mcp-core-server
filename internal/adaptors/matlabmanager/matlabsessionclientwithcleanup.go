// Copyright 2025 The MathWorks, Inc.

package matlabmanager

import (
	"context"

	"github.com/matlab/matlab-mcp-core-server/internal/entities"
)

type matlabSessionClientWithCleanup struct {
	entities.MATLABSessionClient
	sessionCleanup func() error
	detached       bool
}

func newMATLABSessionClientWithCleanup(matlabSessionClient entities.MATLABSessionClient, sessionCleanup func() error) *matlabSessionClientWithCleanup {
	return &matlabSessionClientWithCleanup{
		MATLABSessionClient: matlabSessionClient,
		sessionCleanup:      sessionCleanup,
	}
}

func (c *matlabSessionClientWithCleanup) StopSession(ctx context.Context, sessionLogger entities.Logger) error {
	if c.detached {
		return nil
	}

	_, err := c.Eval(ctx, sessionLogger, entities.EvalRequest{Code: "exit()"})
	if err != nil {
		return err
	}

	return c.sessionCleanup()
}
