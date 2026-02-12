// Copyright 2026 The MathWorks, Inc.

package sessionfile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"syscall"
)

// sessionFilePattern matches PID-namespaced session files: session_vsc<ppid>_ml<mpid>.json
var sessionFilePattern = regexp.MustCompile(`^session_vsc(\d+)_ml(\d+)\.json$`)

// SessionInfo contains the connection details needed to reconnect to a MATLAB session.
// PIDs are encoded in the filename (session_vsc<ppid>_ml<mpid>.json), not in the JSON body.
type SessionInfo struct {
	APIKey  string `json:"api_key"`
	Port    string `json:"port"`
	CertPEM string `json:"cert_pem"`
}

// ScannedSession represents a session file found during directory scanning.
type ScannedSession struct {
	Info      SessionInfo
	ParentPID int
	MatlabPID int
	FilePath  string
}

// sessionFileName builds the PID-namespaced filename for a session file.
func sessionFileName(parentPID, matlabPID int) string {
	return fmt.Sprintf("session_vsc%d_ml%d.json", parentPID, matlabPID)
}

// Write persists session connection details to the given directory with PID-namespaced filename.
func Write(dir string, info SessionInfo, parentPID int, matlabPID int) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create session file directory %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session info: %w", err)
	}

	filePath := filepath.Join(dir, sessionFileName(parentPID, matlabPID))
	if err := os.WriteFile(filePath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write session file %s: %w", filePath, err)
	}

	return nil
}

// ScanSessions scans the given directory for all PID-namespaced session files
// and returns their parsed contents along with the extracted PIDs.
func ScanSessions(dir string) ([]ScannedSession, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read session directory %s: %w", dir, err)
	}

	var sessions []ScannedSession
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		matches := sessionFilePattern.FindStringSubmatch(entry.Name())
		if matches == nil {
			continue
		}

		parentPID, err := strconv.Atoi(matches[1])
		if err != nil {
			continue
		}

		matlabPID, err := strconv.Atoi(matches[2])
		if err != nil {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(filePath) //nolint:gosec // We construct this path
		if err != nil {
			continue
		}

		var info SessionInfo
		if err := json.Unmarshal(data, &info); err != nil {
			continue
		}

		sessions = append(sessions, ScannedSession{
			Info:      info,
			ParentPID: parentPID,
			MatlabPID: matlabPID,
			FilePath:  filePath,
		})
	}

	return sessions, nil
}

// DeleteFile removes a specific session file by its absolute path.
func DeleteFile(filePath string) error {
	err := os.Remove(filePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete session file %s: %w", filePath, err)
	}
	return nil
}

// IsProcessAlive checks whether a process with the given PID is still running.
// Uses signal 0 which doesn't actually send a signal but checks process existence.
func IsProcessAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil
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
