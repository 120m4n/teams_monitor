package detector

import "testing"

// Integration check against the real machine: join/leave a Teams call and run
// `go test -v -run TestIsTeamsCallActive ./pkg/detector`.
func TestIsTeamsCallActive(t *testing.T) {
	if err := InitCOM(); err != nil {
		t.Fatal(err)
	}
	defer ReleaseCOM()

	active, err := IsTeamsCallActive()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Teams call active: %v", active)
}
