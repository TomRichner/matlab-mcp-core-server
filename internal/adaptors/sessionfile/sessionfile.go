// Copyright 2026 The MathWorks, Inc.

package sessionfile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const sessionFileName = "last_matlab_session.json"

// SessionInfo contains the connection details needed to reconnect to a MATLAB session.
type SessionInfo struct {
	APIKey     string `json:"api_key"`
	Port       string `json:"port"`
	CertPEM    string `json:"cert_pem"`
	SessionDir string `json:"session_dir"`
}

// Write persists session connection details to the given directory.
func Write(dir string, info SessionInfo) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create session file directory %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session info: %w", err)
	}

	filePath := filepath.Join(dir, sessionFileName)
	if err := os.WriteFile(filePath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write session file %s: %w", filePath, err)
	}

	return nil
}

// Read loads session connection details from the given directory.
// Returns an error if the file does not exist or cannot be parsed.
func Read(dir string) (SessionInfo, error) {
	filePath := filepath.Join(dir, sessionFileName)

	data, err := os.ReadFile(filePath) //nolint:gosec // We construct this path
	if err != nil {
		return SessionInfo{}, fmt.Errorf("failed to read session file %s: %w", filePath, err)
	}

	var info SessionInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return SessionInfo{}, fmt.Errorf("failed to parse session file %s: %w", filePath, err)
	}

	return info, nil
}

// Delete removes the session file from the given directory.
func Delete(dir string) error {
	filePath := filepath.Join(dir, sessionFileName)
	err := os.Remove(filePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete session file %s: %w", filePath, err)
	}
	return nil
}

// DefaultDir returns the OS-specific default directory for session files.
// Uses os.UserCacheDir() which resolves to:
//   - macOS:   ~/Library/Caches/matlab-mcp/sessions
//   - Linux:   ~/.cache/matlab-mcp/sessions
//   - Windows: C:\Users\<user>\AppData\Local\matlab-mcp\sessions
func DefaultDir() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user cache directory: %w", err)
	}
	return filepath.Join(cacheDir, "matlab-mcp", "sessions"), nil
}

// ResolveDir returns the session file directory to use.
// If flagValue is non-empty, it is used directly. Otherwise, DefaultDir() is used.
func ResolveDir(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	return DefaultDir()
}
