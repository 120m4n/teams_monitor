// Package detector reports whether Microsoft Teams holds an active microphone session.
package detector

import (
	"slices"
	"strings"
)

var teamsExes = []string{"ms-teams.exe", "teams.exe", "msteams.exe"}

// isTeamsProcess reports whether a full image path (e.g. C:\...\ms-teams.exe) is a Teams executable.
func isTeamsProcess(path string) bool {
	return slices.Contains(teamsExes, strings.ToLower(path[strings.LastIndexAny(path, `\/`)+1:]))
}
