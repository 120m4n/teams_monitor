# teams_monitor

Detecta si estás en una llamada de Microsoft Teams en Windows 11 y lo indica con un icono en la bandeja, notificaciones y un anillo de 12 LEDs WS2812B controlado por un ESP8266.

Es un solo binario en Go, sin servicio de Windows, sin drivers y sin plugins de Teams.

## Funcionalidad

- **Detección de llamada**: considera que hay llamada cuando Teams tiene abierta una sesión de captura **activa** en el micrófono de comunicaciones por defecto. No lee audio, solo el estado de la sesión.
- **Bandeja del sistema**: el icono cambia entre inactivo y en llamada. El menú muestra el estado, "Iniciar con Windows" y Salir.
- **Notificaciones** (toast) al empezar y al terminar una llamada.
- **Anillo LED**: se anima durante la llamada y el efecto cambia según su duración (respiración → vúmetro → "auto fantástico" → disco). Conecta por USB serial (`--serial COM16`) o por WiFi/HTTP (`--http http://<ip>/led`). Se pueden usar ambos a la vez.
- **Modo `--headless`**: sin bandeja ni notificaciones; solo detección y anillo.
- **Inicio automático** con la clave `HKCU\...\Run` desde el menú, o con una tarea programada con watchdog (`deploy/install-task.ps1`).
- **Una sola instancia** a la vez, mediante el mutex `Local\TeamsMonitor`.

## Arquitectura

```
 ┌───────────────────────┐   bool cada 1 s   ┌──────────────────────┐  StateIdle / StateInCall
 │ Detector (WASAPI/COM) │ ────────────────► │ Event manager        │ ─────────┬──────────────────┐
 │ pkg/detector          │                   │ pkg/events           │          │                  │
 └───────────────────────┘                   │ polling + debounce   │          ▼                  ▼
                                             └──────────────────────┘  ┌──────────────┐   ┌───────────────────┐
                                                                       │ UI           │   │ Actuadores        │
                                                                       │ pkg/ui       │   │ pkg/actuator      │
                                                                       │ bandeja+toast│   │ Serial  │  HTTP   │
                                                                       └──────────────┘   └────┬────┴────┬────┘
                                                                                               │ USB     │ WiFi
                                                                                               ▼         ▼
                                                                                         ┌───────────────────┐
                                                                                         │ ESP8266 + WS2812B │
                                                                                         │ firmware/esp8266  │
                                                                                         └───────────────────┘
```

### 1. Detector: `pkg/detector`

1. Inicializa COM en el hilo del worker.
2. `GetDefaultAudioEndpoint(eCapture, eCommunications)` → `IAudioSessionManager2` → enumera las sesiones.
3. Para cada sesión obtiene el PID con `IAudioSessionControl2.GetProcessId()` y el nombre del exe con `QueryFullProcessImageNameW`.
4. Hay llamada si `ms-teams.exe`, `Teams.exe` o `msteams.exe` tiene una sesión en estado `AudioSessionStateActive`.

Las sesiones multiproceso se ignoran: go-wca trata `AUDCLNT_S_NO_SINGLE_PROCESS` como error. La lógica pura, la comparación de nombres en `teams.go`, no depende de Windows y tiene tests.

### 2. Event manager: `pkg/events`

Consulta al detector cada `--interval` (1 s por defecto) y solo cambia de estado tras `--debounce` lecturas iguales seguidas (2 por defecto), porque la sesión del micrófono parpadea. Notifica cada transición a los callbacks registrados con `OnStateChange`.

Recibe una función `Check func() (bool, error)`, así que no depende de Windows y se prueba en cualquier SO. COM tiene afinidad de hilo: `Run` llama a `Check` en su propia goroutine, por eso `main` hace `detector.InitCOM()` dentro de esa goroutine.

### 3. Consumidores (independientes entre sí)

- **UI, `pkg/ui`**: bandeja con `getlantern/systray`, notificaciones con `go-toast` e inicio automático en `autostart.go`. Los iconos `icons/{idle,call}.ico` van embebidos en el binario con `go:embed`. La bandeja se suscribe en `onReady` y se sincroniza con `mgr.State()` por si la transición ocurrió antes de que existiera.
- **Actuadores, `pkg/actuator`**: implementan la interfaz `SetCallState(active bool) error`. El protocolo es una línea de texto, `CALL_ON\n` o `CALL_OFF\n`.
  - `SerialActuator` (`go.bug.st/serial`) abre el puerto al primer uso. Si una escritura falla, cierra el puerto, lo reabre y reintenta una vez.
  - `HTTPActuator` envía la misma línea como `POST` con cuerpo `text/plain`.

### 4. Firmware: `firmware/esp8266`

PlatformIO con el core Arduino y Adafruit NeoPixel. Lee líneas por serial a 115200 baudios y, si se compiló con `src/secrets.h`, también acepta `POST /led`. Responde `OK CALL_ON` u `OK CALL_OFF`. El cronómetro de la llamada y la elección del efecto viven en el firmware, así que el PC solo manda las dos transiciones. `FX <n>` fuerza un efecto para pruebas. Pinout, trucos y pruebas en [firmware/esp8266/README.md](firmware/esp8266/README.md).

### Decisiones de diseño

- **Sin servicio de Windows**: los servicios corren en la sesión 0, que no tiene bandeja ni notificaciones y puede no ver la sesión de micrófono de Teams. Por eso la app arranca al iniciar sesión, desde la clave Run o la tarea programada.
- **Al arrancar y al salir** la app manda `CALL_OFF` para que el anillo no quede encendido.

## Estructura

```
cmd/app/            main: flags, log, mutex, cableado del pipeline
cmd/fake_call/      simula una llamada de Teams para pruebas (compilar como ms-teams.exe)
pkg/detector/       detección WASAPI (Windows) + lógica de nombres (portable)
pkg/events/         polling, debounce, máquina de estados
pkg/ui/             bandeja, notificaciones, inicio automático, iconos
pkg/actuator/       actuadores serial y HTTP
firmware/esp8266/   firmware del anillo LED
deploy/             install-task.ps1 (tarea programada con watchdog)
docs/               especificación (README.md) e instalación (INSTALL.md)
tasks/              especificaciones de cada componente
```

## Uso rápido

```powershell
go build -ldflags -H=windowsgui -o teams_monitor.exe ./cmd/app   # en macOS/Linux: GOOS=windows GOARCH=amd64 ...
.\teams_monitor.exe --serial COM16
```

| Flag | Por defecto | |
|---|---|---|
| `--serial COM16` | desactivado | anillo por USB serial |
| `--baud 115200` | 115200 | velocidad serial |
| `--http http://<ip>/led` | desactivado | anillo por WiFi |
| `--headless` | false | sin bandeja ni notificaciones |
| `--interval 1s` | 1s | intervalo de consulta |
| `--debounce 2` | 2 | lecturas iguales para cambiar de estado |

Log: `%LOCALAPPDATA%\teams_monitor\teams_monitor.log`. Instalación e inicio automático en [docs/INSTALL.md](docs/INSTALL.md).

## Simular una llamada (sin Teams)

`cmd/fake_call` abre una sesión de captura en el micrófono de comunicaciones por defecto y la mantiene activa; no lee ni guarda audio. Si se compila con el nombre `ms-teams.exe`, el detector lo toma por Teams. Así se prueba la cadena completa: detector → debounce → bandeja y notificación → serial → firmware.

```powershell
go build -o fake\ms-teams.exe ./cmd/fake_call   # una vez
.\teams_monitor.exe --serial COM16               # la app (otra ventana)
.\fake\ms-teams.exe -for 30s                     # "llamada" de 30 s; Ctrl+C cuelga antes
```

Unos 2 s después de arrancar `ms-teams.exe`, el icono se pone rojo, sale la notificación y el anillo se enciende. Unos 2 s después de colgar, todo vuelve a reposo. Para la llamada falsa, Windows tiene que permitir que las apps de escritorio usen el micrófono (Configuración → Privacidad → Micrófono).

Para validar solo el detector: `.\fake\ms-teams.exe -for 20s` en una ventana y `go test -count=1 -v -run TestIsTeamsCallActive ./pkg/detector` en otra, que debería dar `Teams call active: true`.

## Tests

```powershell
go test ./...                                     # lógica portable (events, actuator, detector/teams)
go test -v -run TestIsTeamsCallActive ./pkg/detector   # solo Windows: consulta real del micrófono
```
