//go:build windows

package ui

import (
	"os"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	runKey   = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValue = "TeamsMonitor"
)

func AutoStartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(runValue)
	return err == nil
}

// SetAutoStart adds/removes the HKCU Run entry. The entry re-launches this exe
// with the current command-line flags (serial port, HTTP URL, ...).
func SetAutoStart(on bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !on {
		if err := k.DeleteValue(runValue); err != nil && err != registry.ErrNotExist {
			return err
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := []string{windows.EscapeArg(exe)}
	for _, a := range os.Args[1:] {
		cmd = append(cmd, windows.EscapeArg(a))
	}
	return k.SetStringValue(runValue, strings.Join(cmd, " "))
}
