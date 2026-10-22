// Teams LED ring: animates a 12-LED WS2812 ring while in a call (CALL_ON), fades it out on CALL_OFF.
// The effect changes with call length (see TIMELINE). "FX <n>" forces one effect, for testing.
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

enum Fx : uint8_t { FX_FADE_IN, FX_BREATHE, FX_VOICE, FX_SCANNER, FX_DISCO, FX_COUNT };
const char *const FX_NAMES[FX_COUNT] = {"fade-in", "breathe", "voice", "scanner", "disco"};

constexpr uint32_t MINUTE = 60000;
constexpr uint32_t FADE_IN_MS = 2000;
constexpr uint32_t FADE_OUT_MS = 800;
constexpr uint32_t FRAME_MS = 20;  // ~50 fps

// Effect by time since CALL_ON. Tune the times here.
struct Step { uint32_t fromMs; Fx fx; };
constexpr Step TIMELINE[] = {
    {0, FX_FADE_IN},
    {FADE_IN_MS, FX_BREATHE},  // calm while the call is short
    {15 * MINUTE, FX_VOICE},
    {30 * MINUTE, FX_SCANNER},
    {45 * MINUTE, FX_DISCO},   // this call is way too long
};

bool on = false;
int8_t forcedFx = -1;  // -1: follow TIMELINE
uint32_t onAt = 0, offAt = 0;  // millis() of the last CALL_ON (or FX) and CALL_OFF
uint8_t master = 0;  // fade-out level applied to every pixel; 0 = ring dark

void px(uint8_t i, uint32_t c) {
  uint8_t r = c >> 16, g = c >> 8, b = c;
  ring.setPixelColor(i, r * master / 255, g * master / 255, b * master / 255);
}

void fill(uint32_t c) {
  for (uint8_t i = 0; i < LED_COUNT; i++) px(i, c);
}

void fadeIn(uint32_t t) {
  uint8_t v = t >= FADE_IN_MS ? 255 : t * 255 / FADE_IN_MS;
  fill(ring.Color(v, 0, 0));
}

void breathe(uint32_t t) {  // ~3 s period; starts at full so it follows fade-in smoothly
  uint8_t v = 60 + ring.sine8(uint8_t(t / 12 + 64)) * 195 / 255;
  fill(ring.Color(v, 0, 0));
}

void voice(uint32_t) {  // fake VU meter: speech-like bursts growing up both sides from LEDs 0/11
  static uint32_t next = 0;
  static uint8_t target = 0;
  static float level = 0;
  uint32_t now = millis();
  if (int32_t(now - next) >= 0) {
    target = random(10) < 3 ? 0 : random(2, LED_COUNT / 2 + 1);
    next = now + random(60, 180);
  }
  level += (target - level) * (target > level ? 0.5f : 0.15f);  // fast attack, slow decay
  for (uint8_t k = 0; k < LED_COUNT / 2; k++) {
    float lit = constrain(level - k, 0.0f, 1.0f);
    uint32_t c = ring.ColorHSV(k * 1500, 255, lit * 255);  // red at the base, orange at the top
    px(k, c);
    px(LED_COUNT - 1 - k, c);
  }
}

void scanner(uint32_t t) {  // "auto fantastico": one red eye sweeping back and forth with a tail
  static uint8_t tail[LED_COUNT];
  uint8_t step = (t / 80) % (2 * LED_COUNT - 2);
  uint8_t pos = step < LED_COUNT ? step : 2 * LED_COUNT - 2 - step;
  for (uint8_t i = 0; i < LED_COUNT; i++) {
    tail[i] = i == pos ? 255 : tail[i] * 7 / 8;
    px(i, ring.Color(tail[i], 0, 0));
  }
}

void disco(uint32_t t) {  // spinning rainbow plus random white flashes
  for (uint8_t i = 0; i < LED_COUNT; i++)
    px(i, ring.gamma32(ring.ColorHSV(i * 65536L / LED_COUNT + t * 40)));
  if (random(100) < 10) px(random(LED_COUNT), 0xFFFFFF);
}

// Returns the effect for `elapsed` ms of call and sets *t to the ms since that effect began.
Fx currentFx(uint32_t elapsed, uint32_t *t) {
  if (forcedFx >= 0) {
    *t = elapsed;
    return Fx(forcedFx);
  }
  size_t i = sizeof(TIMELINE) / sizeof(*TIMELINE) - 1;
  while (TIMELINE[i].fromMs > elapsed) i--;
  *t = elapsed - TIMELINE[i].fromMs;
  return TIMELINE[i].fx;
}

void render() {
  uint32_t now = millis();
  if (on) {
    master = 255;
  } else {
    if (!master) return;  // already dark
    uint32_t dt = now - offAt;
    master = dt >= FADE_OUT_MS ? 0 : 255 - dt * 255 / FADE_OUT_MS;
    if (!master) {
      ring.clear();
      ring.show();
      return;
    }
  }
  uint32_t t;
  switch (currentFx(now - onAt, &t)) {
    case FX_FADE_IN: fadeIn(t); break;
    case FX_BREATHE: breathe(t); break;
    case FX_VOICE: voice(t); break;
    case FX_SCANNER: scanner(t); break;
    default: disco(t); break;
  }
  ring.show();
}

// Returns false for unknown commands.
bool handleCommand(String cmd) {
  cmd.trim();
  if (cmd == "CALL_ON") {
    if (!on) onAt = millis();  // a repeated CALL_ON keeps the call timer running
    on = true;
    forcedFx = -1;
  } else if (cmd == "CALL_OFF") {
    if (on) offAt = millis();
    on = false;
  } else if (cmd.startsWith("FX ") && isDigit(cmd[3])) {
    long n = cmd.substring(3).toInt();
    if (n >= FX_COUNT) return false;
    forcedFx = n;
    on = true;
    onAt = millis();
    cmd += String(" ") + FX_NAMES[n];
  } else {
    return false;
  }
  Serial.println("OK " + cmd);
  return true;
}

void setup() {
  Serial.begin(115200);
  ring.begin();
  ring.setBrightness(BRIGHTNESS);
  ring.clear();
  ring.show();

#if __has_include("secrets.h")
  WiFi.mode(WIFI_STA);
  WiFi.begin(WIFI_SSID, WIFI_PASS);  // non-blocking; the core reconnects on its own
  server.on("/led", HTTP_POST, [] {
    bool ok = handleCommand(server.arg("plain"));
    server.send(ok ? 200 : 400, "text/plain", ok ? "OK\n" : "expected CALL_ON, CALL_OFF or FX <n>\n");
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

  static uint32_t lastFrame = 0;
  if (millis() - lastFrame >= FRAME_MS) {
    lastFrame = millis();
    render();
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
