package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func write(t *testing.T, s string) string {
	p := filepath.Join(t.TempDir(), "teams_monitor.xml")
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadMissing(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "none.xml"))
	if err != nil || c.FX != -1 || c.Actuator.Serial.Baud != 115200 || c.SerialPort() != "" || c.HTTPURL() != "" || c.Web.Listen != "" || c.Web.Cooldown != 30 || c.Web.Duration != 5 || c.Web.FX != 4 || !c.Web.PickFX {
		t.Fatalf("got %+v, %v", c, err)
	}
}

func TestLoadBoth(t *testing.T) {
	c, err := Load(write(t, `<teamsMonitor>
  <processes><exe>ms-teams.exe</exe><exe>zoom.exe</exe></processes>
  <actuator mode="both"><serial port="COM16"/></actuator>
  <wifi ssid="net" password="pw" ip="192.168.1.50" gateway="192.168.1.1" subnet="255.255.255.0"/>
  <fx>3</fx>
  <web listen="127.0.0.1:8080" cooldown="10" duration="8" fx="2" pickfx="false" name="Ana"/>
</teamsMonitor>`))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.Processes, []string{"ms-teams.exe", "zoom.exe"}) {
		t.Errorf("processes = %v", c.Processes)
	}
	if c.SerialPort() != "COM16" || c.Actuator.Serial.Baud != 115200 || c.HTTPURL() != "http://192.168.1.50/led" || c.FX != 3 || c.Web.Listen != "127.0.0.1:8080" || c.Web.Cooldown != 10 || c.Web.Duration != 8 || c.Web.FX != 2 || c.Web.PickFX || c.Web.Name != "Ana" {
		t.Errorf("got %+v", c)
	}
}

func TestLoadHTTPOnly(t *testing.T) {
	c, err := Load(write(t, `<teamsMonitor><actuator mode="http"><serial port="COM16"/></actuator><wifi ip="10.0.0.9"/></teamsMonitor>`))
	if err != nil || c.SerialPort() != "" || c.HTTPURL() != "http://10.0.0.9/led" {
		t.Fatalf("got %+v, %v", c, err)
	}
}

func TestLoadInvalid(t *testing.T) {
	for _, s := range []string{
		`<teamsMonitor><actuator mode="usb"/></teamsMonitor>`,
		`<teamsMonitor><actuator mode="serial"/></teamsMonitor>`,
		`<teamsMonitor><actuator mode="both"><serial port="COM1"/></actuator></teamsMonitor>`,
		`<teamsMonitor>`,
	} {
		if _, err := Load(write(t, s)); err == nil {
			t.Errorf("Load(%s): want error", s)
		}
	}
}

func TestLoadExample(t *testing.T) {
	c, err := Load("../../teams_monitor.example.xml")
	if err != nil {
		t.Fatal(err)
	}
	if c.SerialPort() == "" || c.Web.Listen != "127.0.0.1:8080" || c.Web.Duration != 5 || c.Web.Name == "" {
		t.Errorf("example file drifted from the code: %+v", c)
	}
}
