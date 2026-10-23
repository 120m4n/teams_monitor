// Package detector reports whether Microsoft Teams holds an active microphone session.
package detector

import (
	"slices"
	"strings"
)

// TeamsExes are the executable names watched for a mic session (case-insensitive).
// Set it before the detector starts polling.
var TeamsExes = []string{"ms-teams.exe", "teams.exe", "msteams.exe"}

// isTeamsProcess reports whether a full image path (e.g. C:\...\ms-teams.exe) is a Teams executable.
func isTeamsProcess(path string) bool {
	name := path[strings.LastIndexAny(path, `\/`)+1:]
	return slices.ContainsFunc(TeamsExes, func(exe string) bool { return strings.EqualFold(exe, name) })
}
