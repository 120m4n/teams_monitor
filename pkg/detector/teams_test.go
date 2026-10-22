package detector

import "testing"

func TestIsTeamsProcess(t *testing.T) {
	cases := map[string]bool{
		`C:\Program Files\WindowsApps\MSTeams_24\ms-teams.exe`:       true,
		`C:\Users\x\AppData\Local\Microsoft\Teams\current\Teams.exe`: true,
		`C:\Apps\MSTEAMS.EXE`:     true,
		`ms-teams.exe`:            true,
		`C:\Windows\explorer.exe`: false,
		`C:\Apps\notteams.exe`:    false,
		``:                        false,
	}
	for path, want := range cases {
		if got := isTeamsProcess(path); got != want {
			t.Errorf("isTeamsProcess(%q) = %v, want %v", path, got, want)
		}
	}
}
