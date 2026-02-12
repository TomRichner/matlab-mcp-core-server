// Copyright 2026 The MathWorks, Inc.

package matlabmanager_test

import (
	"testing"

	"github.com/matlab/matlab-mcp-core-server/internal/adaptors/matlabmanager"
	"github.com/matlab/matlab-mcp-core-server/internal/adaptors/matlabmanager/matlabsessionclient/embeddedconnector"
	"github.com/matlab/matlab-mcp-core-server/internal/adaptors/matlabmanager/matlabsessionstore"
	"github.com/matlab/matlab-mcp-core-server/internal/entities"
	"github.com/matlab/matlab-mcp-core-server/internal/testutils"
	mocks "github.com/matlab/matlab-mcp-core-server/mocks/adaptors/matlabmanager"
	entitiesmocks "github.com/matlab/matlab-mcp-core-server/mocks/entities"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestMATLABManager_ReconnectToSession_HappyPath(t *testing.T) {
	// Arrange
	mockLogger := testutils.NewInspectableLogger()

	mockMATLABServices := &mocks.MockMATLABServices{}
	defer mockMATLABServices.AssertExpectations(t)

	mockSessionStore := &mocks.MockMATLABSessionStore{}
	defer mockSessionStore.AssertExpectations(t)

	mockClientFactory := &mocks.MockMATLABSessionClientFactory{}
	defer mockClientFactory.AssertExpectations(t)

	mockSessionClient := &entitiesmocks.MockMATLABSessionClient{}
	defer mockSessionClient.AssertExpectations(t)

	expectedSessionID := entities.SessionID(42)

	connectionDetails := embeddedconnector.ConnectionDetails{
		Host:           "localhost",
		Port:           "31415",
		APIKey:         "test-key",
		CertificatePEM: []byte("test-cert"),
	}

	mockClientFactory.EXPECT().
		New(connectionDetails).
		Return(mockSessionClient, nil).
		Once()

	mockSessionClient.EXPECT().
		Ping(mock.Anything, mockLogger.AsMockArg()).
		Return(entities.PingResponse{IsAlive: true}).
		Once()

	mockSessionStore.EXPECT().
		Add(mock.AnythingOfType("*matlabmanager.matlabSessionClientWithCleanup")).
		Return(expectedSessionID).
		Once()

	manager := matlabmanager.New(mockMATLABServices, mockSessionStore, mockClientFactory, false)
	ctx := t.Context()

	// Act
	sessionID, err := manager.ReconnectToSession(ctx, mockLogger, connectionDetails)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, expectedSessionID, sessionID)
}

func TestMATLABManager_ReconnectToSession_ClientFactoryError(t *testing.T) {
	// Arrange
	mockLogger := testutils.NewInspectableLogger()

	mockMATLABServices := &mocks.MockMATLABServices{}
	defer mockMATLABServices.AssertExpectations(t)

	mockSessionStore := &mocks.MockMATLABSessionStore{}
	defer mockSessionStore.AssertExpectations(t)

	mockClientFactory := &mocks.MockMATLABSessionClientFactory{}
	defer mockClientFactory.AssertExpectations(t)

	connectionDetails := embeddedconnector.ConnectionDetails{
		Host:   "localhost",
		Port:   "31415",
		APIKey: "test-key",
	}

	mockClientFactory.EXPECT().
		New(connectionDetails).
		Return(nil, assert.AnError).
		Once()

	manager := matlabmanager.New(mockMATLABServices, mockSessionStore, mockClientFactory, false)
	ctx := t.Context()

	// Act
	sessionID, err := manager.ReconnectToSession(ctx, mockLogger, connectionDetails)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create client for reconnection")
	assert.Empty(t, sessionID)
}

func TestMATLABManager_ReconnectToSession_PingFails(t *testing.T) {
	// Arrange
	mockLogger := testutils.NewInspectableLogger()

	mockMATLABServices := &mocks.MockMATLABServices{}
	defer mockMATLABServices.AssertExpectations(t)

	mockSessionStore := &mocks.MockMATLABSessionStore{}
	defer mockSessionStore.AssertExpectations(t)

	mockClientFactory := &mocks.MockMATLABSessionClientFactory{}
	defer mockClientFactory.AssertExpectations(t)

	mockSessionClient := &entitiesmocks.MockMATLABSessionClient{}
	defer mockSessionClient.AssertExpectations(t)

	connectionDetails := embeddedconnector.ConnectionDetails{
		Host:   "localhost",
		Port:   "31415",
		APIKey: "test-key",
	}

	mockClientFactory.EXPECT().
		New(connectionDetails).
		Return(mockSessionClient, nil).
		Once()

	mockSessionClient.EXPECT().
		Ping(mock.Anything, mockLogger.AsMockArg()).
		Return(entities.PingResponse{IsAlive: false}).
		Once()

	manager := matlabmanager.New(mockMATLABServices, mockSessionStore, mockClientFactory, false)
	ctx := t.Context()

	// Act
	sessionID, err := manager.ReconnectToSession(ctx, mockLogger, connectionDetails)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not responding to ping")
	assert.Empty(t, sessionID)
}

func TestMATLABManager_LastConnectionDetails_NilBeforeStart(t *testing.T) {
	// Arrange
	mockMATLABServices := &mocks.MockMATLABServices{}
	mockSessionStore := &mocks.MockMATLABSessionStore{}
	mockClientFactory := &mocks.MockMATLABSessionClientFactory{}

	manager := matlabmanager.New(mockMATLABServices, mockSessionStore, mockClientFactory, false)

	// Act
	details := manager.LastConnectionDetails()

	// Assert
	assert.Nil(t, details)
}

func TestMATLABManager_LastConnectionDetails_PopulatedAfterStart(t *testing.T) {
	// Arrange
	mockLogger := testutils.NewInspectableLogger()

	mockMATLABServices := &mocks.MockMATLABServices{}
	defer mockMATLABServices.AssertExpectations(t)

	mockSessionStore := &mocks.MockMATLABSessionStore{}
	defer mockSessionStore.AssertExpectations(t)

	mockClientFactory := &mocks.MockMATLABSessionClientFactory{}
	defer mockClientFactory.AssertExpectations(t)

	mockSessionClient := &entitiesmocks.MockMATLABSessionClient{}

	connectionDetails := embeddedconnector.ConnectionDetails{
		Host:           "localhost",
		Port:           "9876",
		APIKey:         "session-key",
		CertificatePEM: []byte("cert-data"),
	}
	sessionCleanupFunc := func() error { return nil }

	mockMATLABServices.EXPECT().
		StartLocalMATLABSession(mock.Anything, mock.Anything).
		Return(connectionDetails, sessionCleanupFunc, nil).
		Once()

	mockClientFactory.EXPECT().
		New(connectionDetails).
		Return(mockSessionClient, nil).
		Once()

	mockSessionStore.EXPECT().
		Add(mock.AnythingOfType("*matlabmanager.matlabSessionClientWithCleanup")).
		Return(entities.SessionID(1)).
		Once()

	manager := matlabmanager.New(mockMATLABServices, mockSessionStore, mockClientFactory, false)
	ctx := t.Context()

	startRequest := entities.LocalSessionDetails{
		MATLABRoot: "some/root",
	}

	_, err := manager.StartMATLABSession(ctx, mockLogger, startRequest)
	require.NoError(t, err)

	// Act
	details := manager.LastConnectionDetails()

	// Assert
	require.NotNil(t, details)
	assert.Equal(t, "localhost", details.Host)
	assert.Equal(t, "9876", details.Port)
	assert.Equal(t, "session-key", details.APIKey)
	assert.Equal(t, []byte("cert-data"), details.CertificatePEM)
}

func TestMATLABManager_ReconnectToSession_StoresLastConnectionDetails(t *testing.T) {
	// This test verifies the bug fix: ReconnectToSession now stores
	// lastConnectionDetails so that writeSessionFile can persist them.
	mockLogger := testutils.NewInspectableLogger()

	mockMATLABServices := &mocks.MockMATLABServices{}
	defer mockMATLABServices.AssertExpectations(t)

	mockSessionStore := &mocks.MockMATLABSessionStore{}
	defer mockSessionStore.AssertExpectations(t)

	mockClientFactory := &mocks.MockMATLABSessionClientFactory{}
	defer mockClientFactory.AssertExpectations(t)

	mockSessionClient := &entitiesmocks.MockMATLABSessionClient{}
	defer mockSessionClient.AssertExpectations(t)

	connectionDetails := embeddedconnector.ConnectionDetails{
		Host:           "localhost",
		Port:           "12345",
		APIKey:         "reconnect-key",
		CertificatePEM: []byte("reconnect-cert"),
		MatlabPID:      54321,
	}

	mockClientFactory.EXPECT().
		New(connectionDetails).
		Return(mockSessionClient, nil).
		Once()

	mockSessionClient.EXPECT().
		Ping(mock.Anything, mockLogger.AsMockArg()).
		Return(entities.PingResponse{IsAlive: true}).
		Once()

	mockSessionStore.EXPECT().
		Add(mock.AnythingOfType("*matlabmanager.matlabSessionClientWithCleanup")).
		Return(entities.SessionID(7)).
		Once()

	manager := matlabmanager.New(mockMATLABServices, mockSessionStore, mockClientFactory, false)
	ctx := t.Context()

	// Act
	_, err := manager.ReconnectToSession(ctx, mockLogger, connectionDetails)
	require.NoError(t, err)

	// Assert — LastConnectionDetails should be populated after reconnect
	details := manager.LastConnectionDetails()
	require.NotNil(t, details, "LastConnectionDetails must be populated after ReconnectToSession")
	assert.Equal(t, "localhost", details.Host)
	assert.Equal(t, "12345", details.Port)
	assert.Equal(t, "reconnect-key", details.APIKey)
	assert.Equal(t, []byte("reconnect-cert"), details.CertificatePEM)
	assert.Equal(t, 54321, details.MatlabPID)
}

func TestMATLABManager_ReconnectToSession_SetsDetachedMode(t *testing.T) {
	// This test verifies the bug fix: ReconnectToSession now sets detached
	// mode on the session wrapper, preventing adopted MATLAB from being killed.
	mockLogger := testutils.NewInspectableLogger()

	mockMATLABServices := &mocks.MockMATLABServices{}
	defer mockMATLABServices.AssertExpectations(t)

	mockSessionStore := &mocks.MockMATLABSessionStore{}
	defer mockSessionStore.AssertExpectations(t)

	mockClientFactory := &mocks.MockMATLABSessionClientFactory{}
	defer mockClientFactory.AssertExpectations(t)

	mockSessionClient := &entitiesmocks.MockMATLABSessionClient{}
	defer mockSessionClient.AssertExpectations(t)

	connectionDetails := embeddedconnector.ConnectionDetails{
		Host:   "localhost",
		Port:   "31415",
		APIKey: "detach-key",
	}

	mockClientFactory.EXPECT().
		New(connectionDetails).
		Return(mockSessionClient, nil).
		Once()

	mockSessionClient.EXPECT().
		Ping(mock.Anything, mockLogger.AsMockArg()).
		Return(entities.PingResponse{IsAlive: true}).
		Once()

	// Capture the wrapper that ReconnectToSession adds to the store
	var capturedWrapper matlabsessionstore.MATLABSessionClientWithCleanup
	mockSessionStore.EXPECT().
		Add(mock.AnythingOfType("*matlabmanager.matlabSessionClientWithCleanup")).
		Run(func(client matlabsessionstore.MATLABSessionClientWithCleanup) {
			capturedWrapper = client
		}).
		Return(entities.SessionID(8)).
		Once()

	// Create manager with DetachedMode=true
	manager := matlabmanager.New(mockMATLABServices, mockSessionStore, mockClientFactory, true)
	ctx := t.Context()

	// Act
	_, err := manager.ReconnectToSession(ctx, mockLogger, connectionDetails)
	require.NoError(t, err)

	// Assert — StopSession should be a no-op in detached mode
	// If detached were NOT set, StopSession would call Eval("exit()") on mockSessionClient.
	// Since detached=true, StopSession returns nil immediately without calling Eval.
	require.NotNil(t, capturedWrapper)
	stopErr := capturedWrapper.StopSession(ctx, mockLogger)
	require.NoError(t, stopErr)

	// mockSessionClient.AssertExpectations verifies Eval was never called
}
