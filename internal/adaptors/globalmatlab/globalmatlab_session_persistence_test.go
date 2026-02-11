// Copyright 2026 The MathWorks, Inc.

package globalmatlab_test

import (
	"encoding/base64"
	"path/filepath"
	"testing"

	"github.com/matlab/matlab-mcp-core-server/internal/adaptors/globalmatlab"
	"github.com/matlab/matlab-mcp-core-server/internal/adaptors/matlabmanager/matlabsessionclient/embeddedconnector"
	"github.com/matlab/matlab-mcp-core-server/internal/adaptors/sessionfile"
	"github.com/matlab/matlab-mcp-core-server/internal/entities"
	"github.com/matlab/matlab-mcp-core-server/internal/messages"
	"github.com/matlab/matlab-mcp-core-server/internal/testutils"
	configmocks "github.com/matlab/matlab-mcp-core-server/mocks/adaptors/application/config"
	mocks "github.com/matlab/matlab-mcp-core-server/mocks/adaptors/globalmatlab"
	entitiesmocks "github.com/matlab/matlab-mcp-core-server/mocks/entities"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestGlobalMATLAB_Client_ReconnectsFromSessionFile(t *testing.T) {
	// Arrange
	mockLogger := testutils.NewInspectableLogger()

	mockMATLABManager := &mocks.MockMATLABManager{}
	defer mockMATLABManager.AssertExpectations(t)

	mockMATLABRootSelector := &mocks.MockMATLABRootSelector{}
	defer mockMATLABRootSelector.AssertExpectations(t)

	mockMATLABStartingDirSelector := &mocks.MockMATLABStartingDirSelector{}
	defer mockMATLABStartingDirSelector.AssertExpectations(t)

	mockConfigFactory := &mocks.MockConfigFactory{}
	defer mockConfigFactory.AssertExpectations(t)

	expectedSessionClient := &entitiesmocks.MockMATLABSessionClient{}

	ctx := t.Context()
	expectedSessionID := entities.SessionID(99)
	expectedMATLABRoot := filepath.Join("some", "matlab", "root")
	expectedMATLABStartingDir := filepath.Join("some", "starting", "dir")

	// Write a session file for reconnection
	sessionDir := t.TempDir()
	certPEM := []byte("test-certificate-pem")
	info := sessionfile.SessionInfo{
		APIKey:  "saved-api-key",
		Port:    "54321",
		CertPEM: base64.StdEncoding.EncodeToString(certPEM),
	}
	require.NoError(t, sessionfile.Write(sessionDir, info))

	expectedConnectionDetails := embeddedconnector.ConnectionDetails{
		Host:           "localhost",
		Port:           "54321",
		APIKey:         "saved-api-key",
		CertificatePEM: certPEM,
	}

	// Init phase
	mockMATLABRootSelector.EXPECT().
		SelectMATLABRoot(ctx, mockLogger.AsMockArg()).
		Return(expectedMATLABRoot, nil).
		Once()

	mockMATLABStartingDirSelector.EXPECT().
		SelectMatlabStartingDir().
		Return(expectedMATLABStartingDir, nil).
		Once()

	// Reconnect phase (no StartMATLABSession needed!)
	mockMATLABManager.EXPECT().
		ReconnectToSession(ctx, mockLogger.AsMockArg(), expectedConnectionDetails).
		Return(expectedSessionID, nil).
		Once()

	mockMATLABManager.EXPECT().
		GetMATLABSessionClient(ctx, mockLogger.AsMockArg(), expectedSessionID).
		Return(expectedSessionClient, nil).
		Once()

	globalMATLABSession := globalmatlab.New(
		mockMATLABManager,
		mockMATLABRootSelector,
		mockMATLABStartingDirSelector,
		mockConfigFactory,
		globalmatlab.SessionPersistenceConfig{
			UseLastSession: true,
			SessionFileDir: sessionDir,
		},
	)

	// Act
	client, err := globalMATLABSession.Client(ctx, mockLogger)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, expectedSessionClient, client)
}

func TestGlobalMATLAB_Client_FallsBackToNewSessionWhenReconnectFails(t *testing.T) {
	// Arrange
	mockLogger := testutils.NewInspectableLogger()

	mockMATLABManager := &mocks.MockMATLABManager{}
	defer mockMATLABManager.AssertExpectations(t)

	mockMATLABRootSelector := &mocks.MockMATLABRootSelector{}
	defer mockMATLABRootSelector.AssertExpectations(t)

	mockMATLABStartingDirSelector := &mocks.MockMATLABStartingDirSelector{}
	defer mockMATLABStartingDirSelector.AssertExpectations(t)

	mockConfig := &configmocks.MockConfig{}
	defer mockConfig.AssertExpectations(t)

	mockConfigFactory := &mocks.MockConfigFactory{}
	defer mockConfigFactory.AssertExpectations(t)

	expectedSessionClient := &entitiesmocks.MockMATLABSessionClient{}

	ctx := t.Context()
	expectedSessionID := entities.SessionID(77)
	expectedMATLABRoot := filepath.Join("some", "matlab", "root")
	expectedMATLABStartingDir := filepath.Join("some", "starting", "dir")
	shouldShowMATLABDesktop := true

	// Write a session file that will fail to reconnect
	sessionDir := t.TempDir()
	certPEM := []byte("stale-cert")
	info := sessionfile.SessionInfo{
		APIKey:  "stale-key",
		Port:    "11111",
		CertPEM: base64.StdEncoding.EncodeToString(certPEM),
	}
	require.NoError(t, sessionfile.Write(sessionDir, info))

	expectedConnectionDetails := embeddedconnector.ConnectionDetails{
		Host:           "localhost",
		Port:           "11111",
		APIKey:         "stale-key",
		CertificatePEM: certPEM,
	}

	// Init phase
	mockMATLABRootSelector.EXPECT().
		SelectMATLABRoot(ctx, mockLogger.AsMockArg()).
		Return(expectedMATLABRoot, nil).
		Once()

	mockMATLABStartingDirSelector.EXPECT().
		SelectMatlabStartingDir().
		Return(expectedMATLABStartingDir, nil).
		Once()

	// Reconnect fails
	mockMATLABManager.EXPECT().
		ReconnectToSession(ctx, mockLogger.AsMockArg(), expectedConnectionDetails).
		Return(entities.SessionID(0), assert.AnError).
		Once()

	// Falls back to starting new session
	mockConfigFactory.EXPECT().
		Config().
		Return(mockConfig, nil).
		Once()

	mockConfig.EXPECT().
		ShouldShowMATLABDesktop().
		Return(shouldShowMATLABDesktop).
		Once()

	expectedLocalSessionDetails := entities.LocalSessionDetails{
		MATLABRoot:             expectedMATLABRoot,
		IsStartingDirectorySet: true,
		StartingDirectory:      expectedMATLABStartingDir,
		ShowMATLABDesktop:      shouldShowMATLABDesktop,
	}

	mockMATLABManager.EXPECT().
		StartMATLABSession(mock.Anything, mockLogger.AsMockArg(), expectedLocalSessionDetails).
		Return(expectedSessionID, nil).
		Once()

	// writeSessionFile calls LastConnectionDetails
	mockMATLABManager.EXPECT().
		LastConnectionDetails().
		Return(nil).
		Once()

	mockMATLABManager.EXPECT().
		GetMATLABSessionClient(ctx, mockLogger.AsMockArg(), expectedSessionID).
		Return(expectedSessionClient, nil).
		Once()

	globalMATLABSession := globalmatlab.New(
		mockMATLABManager,
		mockMATLABRootSelector,
		mockMATLABStartingDirSelector,
		mockConfigFactory,
		globalmatlab.SessionPersistenceConfig{
			UseLastSession: true,
			SessionFileDir: sessionDir,
		},
	)

	// Act
	client, err := globalMATLABSession.Client(ctx, mockLogger)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, expectedSessionClient, client)

	// Verify the stale session file was deleted
	_, readErr := sessionfile.Read(sessionDir)
	assert.Error(t, readErr, "stale session file should have been deleted")
}

func TestGlobalMATLAB_Client_WritesSessionFileAfterLaunch(t *testing.T) {
	// Arrange
	mockLogger := testutils.NewInspectableLogger()

	mockMATLABManager := &mocks.MockMATLABManager{}
	defer mockMATLABManager.AssertExpectations(t)

	mockMATLABRootSelector := &mocks.MockMATLABRootSelector{}
	defer mockMATLABRootSelector.AssertExpectations(t)

	mockMATLABStartingDirSelector := &mocks.MockMATLABStartingDirSelector{}
	defer mockMATLABStartingDirSelector.AssertExpectations(t)

	mockConfig := &configmocks.MockConfig{}
	defer mockConfig.AssertExpectations(t)

	mockConfigFactory := &mocks.MockConfigFactory{}
	defer mockConfigFactory.AssertExpectations(t)

	expectedSessionClient := &entitiesmocks.MockMATLABSessionClient{}

	ctx := t.Context()
	expectedSessionID := entities.SessionID(55)
	expectedMATLABRoot := filepath.Join("some", "matlab", "root")
	shouldShowMATLABDesktop := false

	sessionDir := t.TempDir()
	certPEM := []byte("new-cert-pem-data")

	// Init phase
	mockMATLABRootSelector.EXPECT().
		SelectMATLABRoot(ctx, mockLogger.AsMockArg()).
		Return(expectedMATLABRoot, nil).
		Once()

	mockMATLABStartingDirSelector.EXPECT().
		SelectMatlabStartingDir().
		Return("", assert.AnError). // starting dir error is non-fatal
		Once()

	// Start new session (no session file to read = empty dir)
	mockConfigFactory.EXPECT().
		Config().
		Return(mockConfig, nil).
		Once()

	mockConfig.EXPECT().
		ShouldShowMATLABDesktop().
		Return(shouldShowMATLABDesktop).
		Once()

	expectedLocalSessionDetails := entities.LocalSessionDetails{
		MATLABRoot:             expectedMATLABRoot,
		IsStartingDirectorySet: false,
		ShowMATLABDesktop:      shouldShowMATLABDesktop,
	}

	mockMATLABManager.EXPECT().
		StartMATLABSession(mock.Anything, mockLogger.AsMockArg(), expectedLocalSessionDetails).
		Return(expectedSessionID, nil).
		Once()

	// writeSessionFile calls LastConnectionDetails
	mockMATLABManager.EXPECT().
		LastConnectionDetails().
		Return(&embeddedconnector.ConnectionDetails{
			Host:           "localhost",
			Port:           "8888",
			APIKey:         "new-api-key",
			CertificatePEM: certPEM,
		}).
		Once()

	mockMATLABManager.EXPECT().
		GetMATLABSessionClient(ctx, mockLogger.AsMockArg(), expectedSessionID).
		Return(expectedSessionClient, nil).
		Once()

	globalMATLABSession := globalmatlab.New(
		mockMATLABManager,
		mockMATLABRootSelector,
		mockMATLABStartingDirSelector,
		mockConfigFactory,
		globalmatlab.SessionPersistenceConfig{
			UseLastSession: true,
			SessionFileDir: sessionDir,
		},
	)

	// Act
	client, err := globalMATLABSession.Client(ctx, mockLogger)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, expectedSessionClient, client)

	// Verify session file was written
	savedInfo, readErr := sessionfile.Read(sessionDir)
	require.NoError(t, readErr)
	assert.Equal(t, "new-api-key", savedInfo.APIKey)
	assert.Equal(t, "8888", savedInfo.Port)

	decodedCert, decodeErr := base64.StdEncoding.DecodeString(savedInfo.CertPEM)
	require.NoError(t, decodeErr)
	assert.Equal(t, certPEM, decodedCert)
}

func TestGlobalMATLAB_Client_NoSessionFileOpsWhenDisabled(t *testing.T) {
	// This test verifies that with SessionPersistenceConfig{} (default/disabled),
	// no reconnect or file write calls are made.
	mockLogger := testutils.NewInspectableLogger()

	mockMATLABManager := &mocks.MockMATLABManager{}
	defer mockMATLABManager.AssertExpectations(t)

	mockMATLABRootSelector := &mocks.MockMATLABRootSelector{}
	defer mockMATLABRootSelector.AssertExpectations(t)

	mockMATLABStartingDirSelector := &mocks.MockMATLABStartingDirSelector{}
	defer mockMATLABStartingDirSelector.AssertExpectations(t)

	mockConfig := &configmocks.MockConfig{}
	defer mockConfig.AssertExpectations(t)

	mockConfigFactory := &mocks.MockConfigFactory{}
	defer mockConfigFactory.AssertExpectations(t)

	expectedSessionClient := &entitiesmocks.MockMATLABSessionClient{}

	ctx := t.Context()
	expectedSessionID := entities.SessionID(10)
	expectedMATLABRoot := filepath.Join("some", "matlab", "root")
	expectedStartingDir := filepath.Join("some", "dir")
	shouldShowMATLABDesktop := true

	mockMATLABRootSelector.EXPECT().
		SelectMATLABRoot(ctx, mockLogger.AsMockArg()).
		Return(expectedMATLABRoot, nil).
		Once()

	mockMATLABStartingDirSelector.EXPECT().
		SelectMatlabStartingDir().
		Return(expectedStartingDir, nil).
		Once()

	mockConfigFactory.EXPECT().
		Config().
		Return(mockConfig, nil).
		Once()

	mockConfig.EXPECT().
		ShouldShowMATLABDesktop().
		Return(shouldShowMATLABDesktop).
		Once()

	mockMATLABManager.EXPECT().
		StartMATLABSession(mock.Anything, mockLogger.AsMockArg(), mock.Anything).
		Return(expectedSessionID, nil).
		Once()

	// NOTE: No ReconnectToSession or LastConnectionDetails expected!

	mockMATLABManager.EXPECT().
		GetMATLABSessionClient(ctx, mockLogger.AsMockArg(), expectedSessionID).
		Return(expectedSessionClient, nil).
		Once()

	globalMATLABSession := globalmatlab.New(
		mockMATLABManager,
		mockMATLABRootSelector,
		mockMATLABStartingDirSelector,
		mockConfigFactory,
		globalmatlab.SessionPersistenceConfig{}, // disabled
	)

	// Act
	client, err := globalMATLABSession.Client(ctx, mockLogger)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, expectedSessionClient, client)
}

func TestGlobalMATLAB_Client_SkipsReconnectWhenNoSessionFile(t *testing.T) {
	// Session persistence enabled but no session file exists — should start new session
	mockLogger := testutils.NewInspectableLogger()

	mockMATLABManager := &mocks.MockMATLABManager{}
	defer mockMATLABManager.AssertExpectations(t)

	mockMATLABRootSelector := &mocks.MockMATLABRootSelector{}
	defer mockMATLABRootSelector.AssertExpectations(t)

	mockMATLABStartingDirSelector := &mocks.MockMATLABStartingDirSelector{}
	defer mockMATLABStartingDirSelector.AssertExpectations(t)

	mockConfig := &configmocks.MockConfig{}
	defer mockConfig.AssertExpectations(t)

	mockConfigFactory := &mocks.MockConfigFactory{}
	defer mockConfigFactory.AssertExpectations(t)

	expectedSessionClient := &entitiesmocks.MockMATLABSessionClient{}

	ctx := t.Context()
	expectedSessionID := entities.SessionID(33)
	expectedMATLABRoot := filepath.Join("some", "matlab", "root")
	shouldShowMATLABDesktop := false

	emptySessionDir := t.TempDir() // no session file in this dir

	mockMATLABRootSelector.EXPECT().
		SelectMATLABRoot(ctx, mockLogger.AsMockArg()).
		Return(expectedMATLABRoot, nil).
		Once()

	mockMATLABStartingDirSelector.EXPECT().
		SelectMatlabStartingDir().
		Return("", assert.AnError).
		Once()

	// No ReconnectToSession because file doesn't exist

	mockConfigFactory.EXPECT().
		Config().
		Return(mockConfig, nil).
		Once()

	mockConfig.EXPECT().
		ShouldShowMATLABDesktop().
		Return(shouldShowMATLABDesktop).
		Once()

	mockMATLABManager.EXPECT().
		StartMATLABSession(mock.Anything, mockLogger.AsMockArg(), mock.Anything).
		Return(expectedSessionID, nil).
		Once()

	mockMATLABManager.EXPECT().
		LastConnectionDetails().
		Return(nil).
		Once()

	mockMATLABManager.EXPECT().
		GetMATLABSessionClient(ctx, mockLogger.AsMockArg(), expectedSessionID).
		Return(expectedSessionClient, nil).
		Once()

	globalMATLABSession := globalmatlab.New(
		mockMATLABManager,
		mockMATLABRootSelector,
		mockMATLABStartingDirSelector,
		mockConfigFactory,
		globalmatlab.SessionPersistenceConfig{
			UseLastSession: true,
			SessionFileDir: emptySessionDir,
		},
	)

	// Act
	client, err := globalMATLABSession.Client(ctx, mockLogger)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, expectedSessionClient, client)
}

func TestNewSessionPersistenceConfig_Disabled(t *testing.T) {
	mockConfig := &configmocks.MockConfig{}
	defer mockConfig.AssertExpectations(t)

	mockConfigFactory := &mocks.MockConfigFactory{}
	defer mockConfigFactory.AssertExpectations(t)

	mockConfigFactory.EXPECT().
		Config().
		Return(mockConfig, nil).
		Once()

	mockConfig.EXPECT().
		UseLastSession().
		Return(false).
		Once()

	cfg := globalmatlab.NewSessionPersistenceConfig(mockConfigFactory)

	assert.False(t, cfg.UseLastSession)
	assert.Empty(t, cfg.SessionFileDir)
}

func TestNewSessionPersistenceConfig_Enabled(t *testing.T) {
	mockConfig := &configmocks.MockConfig{}
	defer mockConfig.AssertExpectations(t)

	mockConfigFactory := &mocks.MockConfigFactory{}
	defer mockConfigFactory.AssertExpectations(t)

	mockConfigFactory.EXPECT().
		Config().
		Return(mockConfig, nil).
		Once()

	mockConfig.EXPECT().
		UseLastSession().
		Return(true).
		Once()

	mockConfig.EXPECT().
		LastSessionFilePath().
		Return(""). // use default
		Once()

	cfg := globalmatlab.NewSessionPersistenceConfig(mockConfigFactory)

	assert.True(t, cfg.UseLastSession)
	assert.NotEmpty(t, cfg.SessionFileDir)
	assert.Contains(t, cfg.SessionFileDir, "matlab-mcp")
}

func TestNewSessionPersistenceConfig_CustomPath(t *testing.T) {
	mockConfig := &configmocks.MockConfig{}
	defer mockConfig.AssertExpectations(t)

	mockConfigFactory := &mocks.MockConfigFactory{}
	defer mockConfigFactory.AssertExpectations(t)

	mockConfigFactory.EXPECT().
		Config().
		Return(mockConfig, nil).
		Once()

	mockConfig.EXPECT().
		UseLastSession().
		Return(true).
		Once()

	mockConfig.EXPECT().
		LastSessionFilePath().
		Return("/custom/session/dir").
		Once()

	cfg := globalmatlab.NewSessionPersistenceConfig(mockConfigFactory)

	assert.True(t, cfg.UseLastSession)
	assert.Equal(t, "/custom/session/dir", cfg.SessionFileDir)
}

func TestNewSessionPersistenceConfig_ConfigError(t *testing.T) {
	mockConfigFactory := &mocks.MockConfigFactory{}
	defer mockConfigFactory.AssertExpectations(t)

	mockConfigFactory.EXPECT().
		Config().
		Return(nil, messages.New_StartupErrors_BadFlag_Error("flag", "value", "reason")).
		Once()

	cfg := globalmatlab.NewSessionPersistenceConfig(mockConfigFactory)

	assert.False(t, cfg.UseLastSession)
	assert.Empty(t, cfg.SessionFileDir)
}
