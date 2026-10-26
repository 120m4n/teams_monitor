// Package config loads teams_monitor.xml. deploy/flash.ps1 reads the same file to build the firmware's secrets.h.
package config

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

type Config struct {
	Processes []string `xml:"processes>exe"` // empty: detector defaults
	Actuator  struct {
		Mode   string `xml:"mode,attr"` // serial | http | both; empty: none (flags only)
		Serial struct {
			Port string `xml:"port,attr"`
			Baud int    `xml:"baud,attr"`
		} `xml:"serial"`
	} `xml:"actuator"`
	WiFi struct {
		SSID     string `xml:"ssid,attr"`
		Password string `xml:"password,attr"`
		IP       string `xml:"ip,attr"` // static IP flashed into the board; the HTTP actuator posts to http://<ip>/led
		Gateway  string `xml:"gateway,attr"`
		Subnet   string `xml:"subnet,attr"`
	} `xml:"wifi"`
	FX  int `xml:"fx"`
	Web struct {
		Listen   string `xml:"listen,attr"`   // e.g. 127.0.0.1:8080; empty: no web page
		Cooldown int    `xml:"cooldown,attr"` // seconds between pings from one client
		Duration int    `xml:"duration,attr"` // seconds one ping plays, 1-60
		FX       int    `xml:"fx,attr"`       // default ping effect, an index into web.Effects
		PickFX   bool   `xml:"pickfx,attr"`   // visitors may pick the effect
		Name     string `xml:"name,attr"`     // whose ring it is, shown on the page
		Timezone string `xml:"timezone,attr"` // IANA zone whose midnight ends a visitor's nick
	} `xml:"web"`
}

// Load reads path. A missing file is not an error: it returns the defaults.
func Load(path string) (*Config, error) {
	c := &Config{FX: -1}
	c.Actuator.Serial.Baud = 115200
	c.Web.Cooldown, c.Web.Duration, c.Web.FX, c.Web.PickFX, c.Web.Timezone = 30, 5, 4, true, "America/Bogota"
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	if err := xml.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	switch c.Actuator.Mode {
	case "", "serial", "http", "both":
	default:
		return nil, fmt.Errorf("%s: actuator mode %q, want serial, http or both", path, c.Actuator.Mode)
	}
	if c.UseSerial() && c.Actuator.Serial.Port == "" {
		return nil, fmt.Errorf("%s: mode %q needs <serial port=\"COMx\"/>", path, c.Actuator.Mode)
	}
	if c.UseHTTP() && c.WiFi.IP == "" {
		return nil, fmt.Errorf("%s: mode %q needs <wifi ip=\"...\"/>", path, c.Actuator.Mode)
	}
	return c, nil
}

func (c *Config) UseSerial() bool { return c.Actuator.Mode == "serial" || c.Actuator.Mode == "both" }
func (c *Config) UseHTTP() bool   { return c.Actuator.Mode == "http" || c.Actuator.Mode == "both" }

// SerialPort is the port to use, or "" if the mode doesn't include serial.
func (c *Config) SerialPort() string {
	if !c.UseSerial() {
		return ""
	}
	return c.Actuator.Serial.Port
}

// HTTPURL is the board's endpoint, or "" if the mode doesn't include HTTP.
func (c *Config) HTTPURL() string {
	if !c.UseHTTP() {
		return ""
	}
	return "http://" + c.WiFi.IP + "/led"
}
