// Copyright 2025 The MathWorks, Inc.

package matlabmanager_test

import (
	"testing"

	"github.com/matlab/matlab-mcp-core-server/internal/adaptors/matlabmanager"
	"github.com/matlab/matlab-mcp-core-server/internal/entities"
	"github.com/matlab/matlab-mcp-core-server/internal/testutils"
	mocks "github.com/matlab/matlab-mcp-core-server/mocks/adaptors/matlabmanager"
	sessionstoremocks "github.com/matlab/matlab-mcp-core-server/mocks/adaptors/matlabmanager/matlabsessionstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMATLABManager_GetMATLABSessionClient_HappyPath(t *testing.T) {
	// Arrange
	mockLogger := testutils.NewInspectableLogger()

	mockMATLABServices := &mocks.MockMATLABServices{}
	defer mockMATLABServices.AssertExpectations(t)

	mockSessionStore := &mocks.MockMATLABSessionStore{}
	defer mockSessionStore.AssertExpectations(t)

	mockClientFactory := &mocks.MockMATLABSessionClientFactory{}
	defer mockClientFactory.AssertExpectations(t)

	mockSessionClient := &sessionstoremocks.MockMATLABSessionClientWithCleanup{}

	expectedSessionID := entities.SessionID(123)
	ctx := t.Context()

	mockSessionStore.EXPECT().
		Get(expectedSessionID).
		Return(mockSessionClient, nil).
		Once()

	mockSessionClient.EXPECT().
		Ping(ctx, mockLogger.AsMockArg()).
		Return(entities.PingResponse{IsAlive: true}).
		Once()

	manager := matlabmanager.New(mockMATLABServices, mockSessionStore, mockClientFactory, false)

	// Act
	client, err := manager.GetMATLABSessionClient(ctx, mockLogger, expectedSessionID)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, mockSessionClient, client)
}

func TestMATLABManager_GetMATLABSessionClient_PingFailure(t *testing.T) {
	// Arrange
	mockLogger := testutils.NewInspectableLogger()

	mockMATLABServices := &mocks.MockMATLABServices{}
	defer mockMATLABServices.AssertExpectations(t)

	mockSessionStore := &mocks.MockMATLABSessionStore{}
	defer mockSessionStore.AssertExpectations(t)

	mockClientFactory := &mocks.MockMATLABSessionClientFactory{}
	defer mockClientFactory.AssertExpectations(t)

	mockSessionClient := &sessionstoremocks.MockMATLABSessionClientWithCleanup{}

	sessionID := entities.SessionID(123)
	ctx := t.Context()

	mockSessionStore.EXPECT().
		Get(sessionID).
		Return(mockSessionClient, nil).
		Once()

	mockSessionClient.EXPECT().
		Ping(ctx, mockLogger.AsMockArg()).
		Return(entities.PingResponse{IsAlive: false}).
		Once()

	manager := matlabmanager.New(mockMATLABServices, mockSessionStore, mockClientFactory, false)

	// Act
	client, err := manager.GetMATLABSessionClient(ctx, mockLogger, sessionID)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not alive")
	assert.Nil(t, client)
}

func TestMATLABManager_GetMATLABSessionClient_SessionStoreError(t *testing.T) {
	// Arrange
	mockLogger := testutils.NewInspectableLogger()

	mockMATLABServices := &mocks.MockMATLABServices{}
	defer mockMATLABServices.AssertExpectations(t)

	mockSessionStore := &mocks.MockMATLABSessionStore{}
	defer mockSessionStore.AssertExpectations(t)

	mockClientFactory := &mocks.MockMATLABSessionClientFactory{}
	defer mockClientFactory.AssertExpectations(t)

	expectedSessionID := entities.SessionID(123)
	ctx := t.Context()
	expectedError := assert.AnError

	mockSessionStore.EXPECT().
		Get(expectedSessionID).
		Return(nil, expectedError).
		Once()

	manager := matlabmanager.New(mockMATLABServices, mockSessionStore, mockClientFactory, false)

	// Act
	client, err := manager.GetMATLABSessionClient(ctx, mockLogger, expectedSessionID)

	// Assert
	require.ErrorIs(t, err, expectedError)
	assert.Nil(t, client)
}
