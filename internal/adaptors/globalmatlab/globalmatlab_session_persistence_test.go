// Copyright 2026 The MathWorks, Inc.

package globalmatlab_test

import (
	"encoding/base64"
	"os"
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

	// Write a session file with our parent PID (so tryReconnectFromSessions finds it)
	// and our own PID as the MATLAB PID (so IsProcessAlive returns true)
	sessionDir := t.TempDir()
	certPEM := []byte("test-certificate-pem")
	myPPID := os.Getppid()
	fakeMatlabPID := os.Getpid() // use our own PID so IsProcessAlive returns true
	info := sessionfile.SessionInfo{
		APIKey:  "saved-api-key",
		Port:    "54321",
		CertPEM: base64.StdEncoding.EncodeToString(certPEM),
	}
	require.NoError(t, sessionfile.Write(sessionDir, info, myPPID, fakeMatlabPID))

	expectedConnectionDetails := embeddedconnector.ConnectionDetails{
		Host:           "localhost",
		Port:           "54321",
		APIKey:         "saved-api-key",
		CertificatePEM: certPEM,
		MatlabPID:      fakeMatlabPID,
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
	myPPID := os.Getppid()
	fakeMatlabPID := os.Getpid() // alive so it gets past IsProcessAlive check
	info := sessionfile.SessionInfo{
		APIKey:  "stale-key",
		Port:    "11111",
		CertPEM: base64.StdEncoding.EncodeToString(certPEM),
	}
	require.NoError(t, sessionfile.Write(sessionDir, info, myPPID, fakeMatlabPID))

	expectedConnectionDetails := embeddedconnector.ConnectionDetails{
		Host:           "localhost",
		Port:           "11111",
		APIKey:         "stale-key",
		CertificatePEM: certPEM,
		MatlabPID:      fakeMatlabPID,
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
	sessions, _ := sessionfile.ScanSessions(sessionDir)
	// The session file for myPPID/fakeMatlabPID should have been deleted after failed reconnect
	for _, s := range sessions {
		assert.NotEqual(t, myPPID, s.ParentPID, "stale session file should have been deleted")
	}
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
	fakeMatlabPID := 99999

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
			MatlabPID:      fakeMatlabPID,
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

	// Verify session file was written with PID-namespaced filename
	sessions, scanErr := sessionfile.ScanSessions(sessionDir)
	require.NoError(t, scanErr)
	require.Len(t, sessions, 1)

	savedInfo := sessions[0].Info
	assert.Equal(t, "new-api-key", savedInfo.APIKey)
	assert.Equal(t, "8888", savedInfo.Port)
	assert.Equal(t, os.Getppid(), sessions[0].ParentPID)
	assert.Equal(t, fakeMatlabPID, sessions[0].MatlabPID)

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

	mockConfig.EXPECT().
		TryToAdopt().
		Return(false).
		Once()

	cfg := globalmatlab.NewSessionPersistenceConfig(mockConfigFactory)

	assert.True(t, cfg.UseLastSession)
	assert.NotEmpty(t, cfg.SessionFileDir)
	assert.Contains(t, cfg.SessionFileDir, "matlab-mcp")
	assert.False(t, cfg.TryToAdopt)
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

	mockConfig.EXPECT().
		TryToAdopt().
		Return(false).
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

func TestNewSessionPersistenceConfig_WithTryToAdopt(t *testing.T) {
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
		Return("").
		Once()

	mockConfig.EXPECT().
		TryToAdopt().
		Return(true).
		Once()

	cfg := globalmatlab.NewSessionPersistenceConfig(mockConfigFactory)

	assert.True(t, cfg.UseLastSession)
	assert.True(t, cfg.TryToAdopt)
}

func TestGlobalMATLAB_Client_GarbageCollectsDeadSessions(t *testing.T) {
	// Write a session file where both PIDs are dead → should be garbage collected
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
	expectedSessionID := entities.SessionID(44)
	expectedMATLABRoot := filepath.Join("some", "matlab", "root")
	shouldShowMATLABDesktop := false

	sessionDir := t.TempDir()

	// Write a stale session file with dead PIDs (use very large PIDs unlikely to exist)
	deadParentPID := 999999998
	deadMatlabPID := 999999999
	info := sessionfile.SessionInfo{
		APIKey:  "dead-key",
		Port:    "22222",
		CertPEM: base64.StdEncoding.EncodeToString([]byte("dead-cert")),
	}
	require.NoError(t, sessionfile.Write(sessionDir, info, deadParentPID, deadMatlabPID))

	// Verify stale file exists before test
	sessionsBefore, _ := sessionfile.ScanSessions(sessionDir)
	require.Len(t, sessionsBefore, 1)

	// Setup mocks for new session launch (since no reconnect will happen)
	mockMATLABRootSelector.EXPECT().
		SelectMATLABRoot(ctx, mockLogger.AsMockArg()).
		Return(expectedMATLABRoot, nil).
		Once()

	mockMATLABStartingDirSelector.EXPECT().
		SelectMatlabStartingDir().
		Return("", assert.AnError).
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

	// Verify the stale session file was garbage-collected
	sessionsAfter, _ := sessionfile.ScanSessions(sessionDir)
	assert.Empty(t, sessionsAfter, "stale session file should have been garbage-collected")
}
