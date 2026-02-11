// Copyright 2026 The MathWorks, Inc.

package sessionfile_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/matlab/matlab-mcp-core-server/internal/adaptors/sessionfile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrite_HappyPath(t *testing.T) {
	dir := t.TempDir()

	info := sessionfile.SessionInfo{
		APIKey:     "test-api-key",
		Port:       "31415",
		CertPEM:    "dGVzdC1jZXJ0LXBlbQ==",
		SessionDir: "/some/session/dir",
	}

	err := sessionfile.Write(dir, info, 12345, 67890)
	require.NoError(t, err)

	// Verify file exists with correct PID-namespaced name
	data, err := os.ReadFile(filepath.Join(dir, "session_vsc12345_ml67890.json"))
	require.NoError(t, err)

	var written sessionfile.SessionInfo
	require.NoError(t, json.Unmarshal(data, &written))
	assert.Equal(t, "test-api-key", written.APIKey)
	assert.Equal(t, "31415", written.Port)
	assert.Equal(t, 12345, written.ParentPID)
	assert.Equal(t, 67890, written.MatlabPID)
}

func TestWrite_CreatesDirectoryIfMissing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")

	info := sessionfile.SessionInfo{
		APIKey: "key",
		Port:   "1234",
	}

	err := sessionfile.Write(dir, info, 111, 222)
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dir, "session_vsc111_ml222.json"))
	require.NoError(t, err)
}

func TestScanSessions_HappyPath(t *testing.T) {
	dir := t.TempDir()

	// Write two session files
	info1 := sessionfile.SessionInfo{APIKey: "key1", Port: "1111"}
	require.NoError(t, sessionfile.Write(dir, info1, 100, 200))

	info2 := sessionfile.SessionInfo{APIKey: "key2", Port: "2222"}
	require.NoError(t, sessionfile.Write(dir, info2, 300, 400))

	sessions, err := sessionfile.ScanSessions(dir)
	require.NoError(t, err)
	assert.Len(t, sessions, 2)

	// Verify PIDs are extracted from filenames
	pids := map[int]int{}
	for _, s := range sessions {
		pids[s.ParentPID] = s.MatlabPID
	}
	assert.Equal(t, 200, pids[100])
	assert.Equal(t, 400, pids[300])
}

func TestScanSessions_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()

	sessions, err := sessionfile.ScanSessions(dir)
	require.NoError(t, err)
	assert.Empty(t, sessions)
}

func TestScanSessions_DirectoryDoesNotExist(t *testing.T) {
	sessions, err := sessionfile.ScanSessions("/nonexistent/dir")
	require.NoError(t, err)
	assert.Nil(t, sessions)
}

func TestScanSessions_SkipsMalformedFilenames(t *testing.T) {
	dir := t.TempDir()

	// Write a valid session
	require.NoError(t, sessionfile.Write(dir, sessionfile.SessionInfo{APIKey: "key"}, 100, 200))

	// Write files that don't match the pattern
	os.WriteFile(filepath.Join(dir, "not_a_session.json"), []byte("{}"), 0o600)
	os.WriteFile(filepath.Join(dir, "session_vsc_ml.json"), []byte("{}"), 0o600)
	os.WriteFile(filepath.Join(dir, "random.txt"), []byte("hello"), 0o600)

	sessions, err := sessionfile.ScanSessions(dir)
	require.NoError(t, err)
	assert.Len(t, sessions, 1)
	assert.Equal(t, 100, sessions[0].ParentPID)
}

func TestScanSessions_SkipsInvalidJSON(t *testing.T) {
	dir := t.TempDir()

	// Write a valid session
	require.NoError(t, sessionfile.Write(dir, sessionfile.SessionInfo{APIKey: "key"}, 100, 200))

	// Write a file with valid name but invalid JSON
	os.WriteFile(filepath.Join(dir, "session_vsc999_ml888.json"), []byte("not json"), 0o600)

	sessions, err := sessionfile.ScanSessions(dir)
	require.NoError(t, err)
	assert.Len(t, sessions, 1)
	assert.Equal(t, 100, sessions[0].ParentPID)
}

func TestDeleteFile_HappyPath(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, sessionfile.Write(dir, sessionfile.SessionInfo{APIKey: "key"}, 100, 200))
	filePath := filepath.Join(dir, "session_vsc100_ml200.json")

	err := sessionfile.DeleteFile(filePath)
	require.NoError(t, err)

	_, err = os.Stat(filePath)
	assert.True(t, os.IsNotExist(err))
}

func TestDeleteFile_FileDoesNotExist(t *testing.T) {
	err := sessionfile.DeleteFile("/nonexistent/file.json")
	require.NoError(t, err) // should not error
}

func TestIsProcessAlive_CurrentProcess(t *testing.T) {
	// Our own PID should be alive
	assert.True(t, sessionfile.IsProcessAlive(os.Getpid()))
}

func TestIsProcessAlive_DeadProcess(t *testing.T) {
	// PID 0 is special but a very large PID should not exist
	assert.False(t, sessionfile.IsProcessAlive(999999999))
}

func TestDefaultDir_ReturnsNonEmptyPath(t *testing.T) {
	dir, err := sessionfile.DefaultDir()
	require.NoError(t, err)
	assert.NotEmpty(t, dir)
	assert.Contains(t, dir, "matlab-mcp")
	assert.Contains(t, dir, "sessions")
}

func TestResolveDir_UsesProvidedValue(t *testing.T) {
	dir, err := sessionfile.ResolveDir("/custom/path")
	require.NoError(t, err)
	assert.Equal(t, "/custom/path", dir)
}

func TestResolveDir_FallsBackToDefault(t *testing.T) {
	dir, err := sessionfile.ResolveDir("")
	require.NoError(t, err)

	expected, err := sessionfile.DefaultDir()
	require.NoError(t, err)
	assert.Equal(t, expected, dir)
}
