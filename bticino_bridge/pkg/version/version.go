// Package version provides centralized version management for BTicino Enhanced Bridge
package version

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	// Version is the fallback default (should be overridden by VERSION file)
	Version = "0.0.0"

	// GitCommit will be set at build time if available
	GitCommit = "unknown"

	// BuildDate will be set at build time
	BuildDate = "unknown"
)

// GetVersion returns the current version (always reads from VERSION file)
func GetVersion() string {
	if v, err := GetVersionFromFile(); err == nil && v != "" {
		return v
	}
	return Version
}

// GetFullVersion returns version with build info
func GetFullVersion() string {
	return fmt.Sprintf("%s (commit: %s, built: %s)", GetVersion(), GitCommit, BuildDate)
}

// GetVersionFromFile reads version from VERSION file
func GetVersionFromFile() (string, error) {
	// Get the directory of the current executable or project root
	execDir, err := os.Executable()
	if err != nil {
		// Fallback to current working directory
		execDir, _ = os.Getwd()
	} else {
		execDir = filepath.Dir(execDir)
	}

	// Try to find VERSION file in project structure
	versionPaths := []string{
		filepath.Join(execDir, "VERSION"),
		filepath.Join(execDir, "..", "VERSION"),
		filepath.Join(execDir, "..", "..", "VERSION"),
		"VERSION",
	}

	for _, versionPath := range versionPaths {
		if data, err := ioutil.ReadFile(versionPath); err == nil {
			version := strings.TrimSpace(string(data))
			if version != "" {
				return version, nil
			}
		}
	}

	return Version, nil
}

// InitVersion initializes version from file if available
func InitVersion() {
	if fileVersion, err := GetVersionFromFile(); err == nil && fileVersion != "" {
		Version = fileVersion
	}

	// Set build date if not set
	if BuildDate == "unknown" {
		BuildDate = time.Now().Format("2006-01-02T15:04:05Z")
	}
}

// BinaryName returns the standard binary name
func BinaryName() string {
	return "bticino-bridge"
}

// BinaryNameArm returns the standard ARM binary name
func BinaryNameArm() string {
	return "bticino-bridge-arm"
}
