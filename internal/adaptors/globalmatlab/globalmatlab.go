// Copyright 2025-2026 The MathWorks, Inc.

package globalmatlab

import (
	"context"
	"encoding/base64"
	"fmt"
	"sync"

	"github.com/matlab/matlab-mcp-core-server/internal/adaptors/application/config"
	"github.com/matlab/matlab-mcp-core-server/internal/adaptors/matlabmanager/matlabsessionclient/embeddedconnector"
	"github.com/matlab/matlab-mcp-core-server/internal/adaptors/sessionfile"
	"github.com/matlab/matlab-mcp-core-server/internal/entities"
	"github.com/matlab/matlab-mcp-core-server/internal/messages"
)

type ConfigFactory interface {
	Config() (config.Config, messages.Error)
}

type MATLABManager interface {
	StartMATLABSession(ctx context.Context, sessionLogger entities.Logger, startRequest entities.SessionDetails) (entities.SessionID, error)
	StopMATLABSession(ctx context.Context, sessionLogger entities.Logger, sessionID entities.SessionID) error
	GetMATLABSessionClient(ctx context.Context, sessionLogger entities.Logger, sessionID entities.SessionID) (entities.MATLABSessionClient, error)
	ReconnectToSession(ctx context.Context, sessionLogger entities.Logger, connectionDetails embeddedconnector.ConnectionDetails) (entities.SessionID, error)
	LastConnectionDetails() *embeddedconnector.ConnectionDetails
}

type MATLABRootSelector interface {
	SelectMATLABRoot(ctx context.Context, logger entities.Logger) (string, error)
}

type MATLABStartingDirSelector interface {
	SelectMatlabStartingDir() (string, error)
}

// SessionPersistenceConfig holds the resolved session persistence settings.
// A zero-value disables the feature (UseLastSession=false).
type SessionPersistenceConfig struct {
	UseLastSession bool
	SessionFileDir string
}

// NewSessionPersistenceConfig creates a SessionPersistenceConfig from the application config.
// This is intended to be called via dependency injection (wire).
func NewSessionPersistenceConfig(configFactory ConfigFactory) SessionPersistenceConfig {
	cfg, err := configFactory.Config()
	if err != nil {
		return SessionPersistenceConfig{}
	}

	if !cfg.UseLastSession() {
		return SessionPersistenceConfig{}
	}

	dir, resolveErr := sessionfile.ResolveDir(cfg.LastSessionFilePath())
	if resolveErr != nil {
		return SessionPersistenceConfig{}
	}

	return SessionPersistenceConfig{
		UseLastSession: true,
		SessionFileDir: dir,
	}
}

type GlobalMATLAB struct {
	matlabManager             MATLABManager
	matlabRootSelector        MATLABRootSelector
	matlabStartingDirSelector MATLABStartingDirSelector
	configFactory             ConfigFactory

	lock *sync.Mutex

	initOnce  *sync.Once
	initError error

	matlabRoot        string
	matlabStartingDir string
	sessionID         entities.SessionID

	// Session persistence
	useLastSession bool
	sessionFileDir string
}

func New(
	matlabManager MATLABManager,
	matlabRootSelector MATLABRootSelector,
	matlabStartingDirSelector MATLABStartingDirSelector,
	configFactory ConfigFactory,
	sessionPersistence SessionPersistenceConfig,
) *GlobalMATLAB {
	return &GlobalMATLAB{
		matlabManager:             matlabManager,
		matlabRootSelector:        matlabRootSelector,
		matlabStartingDirSelector: matlabStartingDirSelector,
		configFactory:             configFactory,

		lock:     &sync.Mutex{},
		initOnce: &sync.Once{},

		useLastSession: sessionPersistence.UseLastSession,
		sessionFileDir: sessionPersistence.SessionFileDir,
	}
}

func (g *GlobalMATLAB) Client(ctx context.Context, logger entities.Logger) (entities.MATLABSessionClient, error) {
	g.lock.Lock()
	defer g.lock.Unlock()

	g.initOnce.Do(func() {
		err := g.initializeStartupConfig(ctx, logger)
		if err != nil {
			g.initError = err
		}
	})

	if g.initError != nil {
		return nil, g.initError
	}

	return g.getOrCreateClient(ctx, logger)
}

func (g *GlobalMATLAB) getOrCreateClient(ctx context.Context, logger entities.Logger) (entities.MATLABSessionClient, error) {
	var sessionIDZeroValue entities.SessionID

	// Try to reconnect from session file if enabled and no current session
	if g.sessionID == sessionIDZeroValue && g.useLastSession {
		if g.tryReconnectFromFile(ctx, logger) {
			logger.Info("Reconnected to existing MATLAB session from session file")
		}
	}

	// Start MATLAB if we don't have a session
	if g.sessionID == sessionIDZeroValue {
		if err := g.startNewSession(ctx, logger); err != nil {
			g.initError = err
			return nil, err
		}
		// Persist session details after successful launch
		g.writeSessionFile(logger)
	}

	// Try to get the client
	client, err := g.matlabManager.GetMATLABSessionClient(ctx, logger, g.sessionID)
	if err != nil {
		// Retry: stop old session and start a new one
		if stopErr := g.matlabManager.StopMATLABSession(ctx, logger, g.sessionID); stopErr != nil {
			logger.WithError(stopErr).Warn("failed to stop MATLAB session")
		}

		if err := g.startNewSession(ctx, logger); err != nil {
			g.initError = err
			return nil, err
		}
		// Persist session details after retry launch
		g.writeSessionFile(logger)

		return g.matlabManager.GetMATLABSessionClient(ctx, logger, g.sessionID)
	}

	return client, nil
}

func (g *GlobalMATLAB) startNewSession(ctx context.Context, logger entities.Logger) error {
	config, messagesErr := g.configFactory.Config()
	if messagesErr != nil {
		return messagesErr
	}

	sessionID, err := g.matlabManager.StartMATLABSession(ctx, logger, entities.LocalSessionDetails{
		MATLABRoot:             g.matlabRoot,
		IsStartingDirectorySet: g.matlabStartingDir != "",
		StartingDirectory:      g.matlabStartingDir,
		ShowMATLABDesktop:      config.ShouldShowMATLABDesktop(),
	})
	if err != nil {
		return err
	}

	g.sessionID = sessionID
	return nil
}

func (g *GlobalMATLAB) initializeStartupConfig(ctx context.Context, logger entities.Logger) error {
	matlabRoot, err := g.matlabRootSelector.SelectMATLABRoot(ctx, logger)
	if err != nil {
		return err
	}

	g.matlabRoot = matlabRoot

	matlabStartingDirectory, err := g.matlabStartingDirSelector.SelectMatlabStartingDir()
	if err != nil {
		logger.WithError(err).Warn("failed to determine MATLAB starting directory, proceeding without one")
		return nil
	}

	g.matlabStartingDir = matlabStartingDirectory
	return nil
}

// tryReconnectFromFile attempts to read saved session details and reconnect to an existing MATLAB.
// Returns true if reconnection was successful.
func (g *GlobalMATLAB) tryReconnectFromFile(ctx context.Context, logger entities.Logger) bool {
	if g.sessionFileDir == "" {
		logger.Debug("No session file directory configured, skipping reconnection attempt")
		return false
	}

	info, err := sessionfile.Read(g.sessionFileDir)
	if err != nil {
		logger.WithError(err).Debug("Could not read session file, will start new session")
		return false
	}

	// Decode the base64-encoded certificate PEM
	certPEM, err := base64.StdEncoding.DecodeString(info.CertPEM)
	if err != nil {
		logger.WithError(err).Warn("Failed to decode certificate from session file, will start new session")
		_ = sessionfile.Delete(g.sessionFileDir)
		return false
	}

	connectionDetails := embeddedconnector.ConnectionDetails{
		Host:           "localhost",
		Port:           info.Port,
		APIKey:         info.APIKey,
		CertificatePEM: certPEM,
	}

	sessionID, err := g.matlabManager.ReconnectToSession(ctx, logger, connectionDetails)
	if err != nil {
		logger.WithError(err).Info("Failed to reconnect to existing MATLAB session, will start new session")
		_ = sessionfile.Delete(g.sessionFileDir)
		return false
	}

	g.sessionID = sessionID
	return true
}

// writeSessionFile persists connection details for the current session.
// This is a best-effort operation; failures are logged but don't prevent operation.
func (g *GlobalMATLAB) writeSessionFile(logger entities.Logger) {
	if !g.useLastSession || g.sessionFileDir == "" {
		return
	}

	details := g.matlabManager.LastConnectionDetails()
	if details == nil {
		logger.Warn("No connection details available, session file not written")
		return
	}

	info := sessionfile.SessionInfo{
		APIKey:  details.APIKey,
		Port:    details.Port,
		CertPEM: base64.StdEncoding.EncodeToString(details.CertificatePEM),
	}

	if err := sessionfile.Write(g.sessionFileDir, info); err != nil {
		logger.WithError(err).Warn("Failed to write session file")
		return
	}

	logger.Info(fmt.Sprintf("Session details persisted to session file in %s", g.sessionFileDir))
}
