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

	err := sessionfile.Write(dir, info)
	require.NoError(t, err)

	// Verify file exists and has correct content
	data, err := os.ReadFile(filepath.Join(dir, "last_matlab_session.json"))
	require.NoError(t, err)

	var written sessionfile.SessionInfo
	require.NoError(t, json.Unmarshal(data, &written))
	assert.Equal(t, info, written)
}

func TestWrite_CreatesDirectoryIfMissing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")

	info := sessionfile.SessionInfo{
		APIKey: "key",
		Port:   "1234",
	}

	err := sessionfile.Write(dir, info)
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dir, "last_matlab_session.json"))
	require.NoError(t, err)
}

func TestRead_HappyPath(t *testing.T) {
	dir := t.TempDir()

	expected := sessionfile.SessionInfo{
		APIKey:     "my-key",
		Port:       "9999",
		CertPEM:    "Y2VydA==",
		SessionDir: "/tmp/session",
	}

	err := sessionfile.Write(dir, expected)
	require.NoError(t, err)

	actual, err := sessionfile.Read(dir)
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestRead_FileDoesNotExist(t *testing.T) {
	dir := t.TempDir()

	_, err := sessionfile.Read(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read session file")
}

func TestRead_InvalidJSON(t *testing.T) {
	dir := t.TempDir()

	err := os.WriteFile(filepath.Join(dir, "last_matlab_session.json"), []byte("not json"), 0o600)
	require.NoError(t, err)

	_, err = sessionfile.Read(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse session file")
}

func TestDelete_HappyPath(t *testing.T) {
	dir := t.TempDir()

	info := sessionfile.SessionInfo{APIKey: "key", Port: "1234"}
	require.NoError(t, sessionfile.Write(dir, info))

	err := sessionfile.Delete(dir)
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dir, "last_matlab_session.json"))
	assert.True(t, os.IsNotExist(err))
}

func TestDelete_FileDoesNotExist(t *testing.T) {
	dir := t.TempDir()

	err := sessionfile.Delete(dir)
	require.NoError(t, err) // should not error
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
