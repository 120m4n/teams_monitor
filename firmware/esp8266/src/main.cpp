// Teams LED ring: lights a 12-LED WS2812 ring on CALL_ON, clears it on CALL_OFF.
// Commands arrive as lines over USB serial (115200) or as the body of POST /led.
#include <Arduino.h>
#include <Adafruit_NeoPixel.h>

#if __has_include("secrets.h")
#include "secrets.h"
#include <ESP8266WiFi.h>
#include <ESP8266WebServer.h>
ESP8266WebServer server(80);
#endif

constexpr uint8_t LED_PIN = D4;  // GPIO2: also the onboard LED, which will flicker with data
constexpr uint8_t LED_COUNT = 12;
constexpr uint8_t BRIGHTNESS = 80;  // 0-255; full white on 12 LEDs draws ~720 mA

Adafruit_NeoPixel ring(LED_COUNT, LED_PIN, NEO_GRB + NEO_KHZ800);

void setRing(bool on) {
  ring.fill(on ? ring.Color(255, 0, 0) : 0);
  ring.show();
}

// Returns false for unknown commands.
bool handleCommand(String cmd) {
  cmd.trim();
  if (cmd == "CALL_ON") setRing(true);
  else if (cmd == "CALL_OFF") setRing(false);
  else return false;
  Serial.println("OK " + cmd);
  return true;
}

void setup() {
  Serial.begin(115200);
  ring.begin();
  ring.setBrightness(BRIGHTNESS);
  setRing(false);

#if __has_include("secrets.h")
  WiFi.mode(WIFI_STA);
  WiFi.begin(WIFI_SSID, WIFI_PASS);  // non-blocking; the core reconnects on its own
  server.on("/led", HTTP_POST, [] {
    bool ok = handleCommand(server.arg("plain"));
    server.send(ok ? 200 : 400, "text/plain", ok ? "OK\n" : "expected CALL_ON or CALL_OFF\n");
  });
  server.begin();
#endif
  Serial.println("teams_led ready");
}

void loop() {
  static String line;
  while (Serial.available()) {
    char c = Serial.read();
    if (c == '\n') {
      if (!handleCommand(line)) Serial.println("ERR " + line);
      line = "";
    } else if (line.length() < 32) {
      line += c;
    }
  }

#if __has_include("secrets.h")
  static bool announced = false;
  if (WiFi.status() == WL_CONNECTED && !announced) {
    Serial.println("HTTP: POST http://" + WiFi.localIP().toString() + "/led");
    announced = true;
  } else if (WiFi.status() != WL_CONNECTED) {
    announced = false;
  }
  server.handleClient();
#endif
}
