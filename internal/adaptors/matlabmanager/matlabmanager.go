// Copyright 2025 The MathWorks, Inc.

package matlabmanager

import (
	"github.com/matlab/matlab-mcp-core-server/internal/adaptors/matlabmanager/matlabservices/datatypes"
	"github.com/matlab/matlab-mcp-core-server/internal/adaptors/matlabmanager/matlabsessionclient/embeddedconnector"
	"github.com/matlab/matlab-mcp-core-server/internal/adaptors/matlabmanager/matlabsessionstore"
	"github.com/matlab/matlab-mcp-core-server/internal/entities"
)

type MATLABServices interface {
	ListDiscoveredMatlabInfo(logger entities.Logger) datatypes.ListMatlabInfo
	StartLocalMATLABSession(logger entities.Logger, request datatypes.LocalSessionDetails) (embeddedconnector.ConnectionDetails, func() error, error)
}

type MATLABSessionStore interface {
	Add(client matlabsessionstore.MATLABSessionClientWithCleanup) entities.SessionID
	Get(sessionID entities.SessionID) (matlabsessionstore.MATLABSessionClientWithCleanup, error)
	Remove(sessionID entities.SessionID)
}

type MATLABSessionClientFactory interface {
	New(endpoint embeddedconnector.ConnectionDetails) (entities.MATLABSessionClient, error)
}

// DetachedMode controls whether MATLAB sessions survive server shutdown.
// When true, StopSession() is a no-op, allowing the next server instance to reconnect.
type DetachedMode bool

type MATLABManager struct {
	matlabServices        MATLABServices
	sessionStore          MATLABSessionStore
	clientFactory         MATLABSessionClientFactory
	lastConnectionDetails *embeddedconnector.ConnectionDetails
	detachedMode          bool
}

var _ entities.MATLABManager = (*MATLABManager)(nil)

func New(
	matlabServices MATLABServices,
	sessionStore MATLABSessionStore,
	clientFactory MATLABSessionClientFactory,
	detachedMode DetachedMode,
) *MATLABManager {
	return &MATLABManager{
		matlabServices: matlabServices,
		sessionStore:   sessionStore,
		clientFactory:  clientFactory,
		detachedMode:   bool(detachedMode),
	}
}

// LastConnectionDetails returns the connection details from the most recent StartMATLABSession call.
// Returns nil if no session has been started.
func (m *MATLABManager) LastConnectionDetails() *embeddedconnector.ConnectionDetails {
	return m.lastConnectionDetails
}
