// Copyright 2026 The MathWorks, Inc.

package matlabmanager

import (
	"context"
	"fmt"

	"github.com/matlab/matlab-mcp-core-server/internal/adaptors/matlabmanager/matlabsessionclient/embeddedconnector"
	"github.com/matlab/matlab-mcp-core-server/internal/entities"
)

// ReconnectToSession creates a client from existing connection details (without launching MATLAB)
// and registers it in the session store. This is used to reconnect to a MATLAB that was started
// by a previous server process.
func (m *MATLABManager) ReconnectToSession(ctx context.Context, sessionLogger entities.Logger, connectionDetails embeddedconnector.ConnectionDetails) (entities.SessionID, error) {
	sessionLogger.Info(fmt.Sprintf("Attempting to reconnect to existing MATLAB session on port %s", connectionDetails.Port))

	client, err := m.clientFactory.New(connectionDetails)
	if err != nil {
		return 0, fmt.Errorf("failed to create client for reconnection: %w", err)
	}

	// Verify the session is still alive
	pingResponse := client.Ping(ctx, sessionLogger)
	if !pingResponse.IsAlive {
		return 0, fmt.Errorf("MATLAB session is not responding to ping")
	}

	// Register with a no-op cleanup since we didn't launch this process
	sessionClient := newMATLABSessionClientWithCleanup(client, func() error {
		return nil
	})

	sessionID := m.sessionStore.Add(sessionClient)
	sessionLogger.Info(fmt.Sprintf("Successfully reconnected to existing MATLAB session with session_id %d", sessionID))
	return sessionID, nil
}
