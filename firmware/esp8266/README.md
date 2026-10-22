# Firmware del anillo LED (ESP8266)

Anima un anillo de 12 LEDs WS2812B (WCMCU-2812B-12) durante la llamada (`CALL_ON`) y lo apaga con un fade-out de 0,8 s (`CALL_OFF`). Los comandos llegan como líneas por USB serial (115200 baudios) o como cuerpo de `POST /led` por WiFi.

## Efectos según la duración de la llamada

| Desde | `FX` | Efecto |
|---|---|---|
| 0 s | 0 | **fade-in**: el rojo sube de 0 al máximo en 2 s |
| 2 s | 1 | **breathe**: rojo que "respira", con un ciclo de unos 3 s |
| 15 min | 2 | **voice**: vúmetro simulado; ráfagas tipo voz que suben por ambos lados desde los LEDs 0 y 11, de rojo a naranja |
| 30 min | 3 | **scanner**: el "auto fantástico", un ojo rojo que va y vuelve con estela |
| 45 min | 4 | **disco**: arcoíris girando con destellos blancos |

- Los tiempos se ajustan en `TIMELINE`, en `src/main.cpp`. Los tiempos del fade están en `FADE_IN_MS` y `FADE_OUT_MS`.
- El efecto voice **no escucha audio**: es aleatorio y solo lo imita.
- Si llega un `CALL_ON` repetido con la llamada ya en curso, el cronómetro sigue corriendo.

### Comandos

| Comando | Respuesta | Qué hace |
|---|---|---|
| `CALL_ON` | `OK CALL_ON` | empieza la llamada y sigue el timeline |
| `CALL_OFF` | `OK CALL_OFF` | fade-out y apagado |
| `FX <0-4>` | `OK FX 3 scanner` | fuerza un efecto (para pruebas) hasta el siguiente `CALL_ON` o `CALL_OFF` |
| cualquier otro | `ERR <texto>` | se ignora (por HTTP devuelve 400) |

## Pinout

```
 Anillo WS2812B (WCMCU-2812B-12)        NodeMCU / D1 mini (ESP8266)
 ───────────────────────────────        ───────────────────────────
  DI / DIN / IN   ───────────────────►  D4  (GPIO2)
  5V / VCC        ───────────────────►  VIN o VU (NodeMCU)  |  5V (D1 mini)
  GND             ───────────────────►  G / GND
  DO / DOUT       ───  (sin conectar)
```

- Conecta GND y 5V antes que el cable de datos.
- DOUT solo sirve para encadenar otro anillo.
- Recomendado: una resistencia de 300–500 Ω en serie en DIN y un condensador de 470–1000 µF entre 5V y GND.
- Para cambiar el pin, el número de LEDs o el brillo, edita `LED_PIN`, `LED_COUNT` y `BRIGHTNESS` en `src/main.cpp`.

## Si el anillo no enciende

Causas más probables, en orden:

1. **Cable de datos en DOUT en vez de DIN.** Es el error más común. El anillo trae dos pads parecidos; los datos deben ir a **DI/DIN**. Guíate por la serigrafía del pad, no por el color del cable.
2. **El 5V no llega al anillo.** Una placa con chip CH340 probablemente es una NodeMCU "LoLin v3":
   - En esa placa el 5V del USB sale por el pin **VU**.
   - Si el pin **VIN** no te da voltaje, mueve el cable a **VU**.
   - Para salir de dudas, mide con multímetro entre VCC y GND del anillo mientras la placa está conectada por USB.
3. **Nivel de señal.** El ESP8266 manda los datos a 3.3 V y algunos anillos alimentados a 5 V no los reconocen. Prueba a alimentar el anillo desde **3V3** en lugar de 5V. Al brillo configurado consume unos 75 mA, así que el pin 3V3 lo aguanta. La alternativa es un conversor de nivel como el 74AHCT125.
4. **Soldaduras o conexiones flojas** en los pads del anillo, que son muy pequeños.

## Trucos

- **¿Falla el firmware o el cableado?** El LED azul de la placa también está en D4. Si parpadea al mandar `CALL_ON`, el firmware está enviando datos y el problema es el cableado o la alimentación del anillo. Si no cambia nada, el problema está en el firmware.
- **Puerto ocupado.** Solo un programa puede abrir el COM a la vez: cierra `pio device monitor`, Arduino IDE u otras sesiones de PowerShell antes de lanzar `teams_monitor.exe`.
- **El ESP8266 se reinicia al abrir el puerto** porque DTR y RTS están conectados al reset. Espera unos 2 s antes de mandar comandos. Las pruebas de abajo ya lo hacen.
- **Placa D1 mini**: cambia `board = nodemcuv2` por `board = d1_mini` en `platformio.ini`.
- **WiFi**: copia `src/secrets.example.h` a `src/secrets.h`, pon el SSID y la contraseña (solo redes de 2.4 GHz) y vuelve a flashear. Sin ese archivo el firmware solo acepta serial. Reserva la IP de la placa en el router para que no cambie.
- **PlatformIO sin instalar**: `uv tool install platformio`, o `pip install platformio`.

## Compilar y flashear

```powershell
cd firmware\esp8266
pio run -t upload --upload-port COM16   # compila y flashea
pio device monitor -p COM16             # muestra "teams_led ready" y, con WiFi, la URL HTTP (Ctrl+C para salir)
```

## Pruebas desde PowerShell

**Buscar el puerto COM de la placa:**

```powershell
Get-CimInstance Win32_PnPEntity | Where-Object Name -match '\(COM\d+\)' | Select-Object Name
```

**Encender, esperar 3 s y apagar por serial:**

```powershell
$p = New-Object System.IO.Ports.SerialPort COM16,115200
$p.Open(); Start-Sleep 2; $p.ReadExisting() | Out-Null
$p.WriteLine("CALL_ON");  Start-Sleep 3; $p.ReadExisting()   # -> OK CALL_ON
$p.WriteLine("CALL_OFF"); Start-Sleep 1; $p.ReadExisting()   # -> OK CALL_OFF
$p.Close()
```

Un comando desconocido devuelve `ERR <texto>`.

**Ver cada efecto sin esperar 45 minutos (5 s por efecto):**

```powershell
$p = New-Object System.IO.Ports.SerialPort COM16,115200
$p.Open(); Start-Sleep 2; $p.ReadExisting() | Out-Null
0..4 | ForEach-Object { $p.WriteLine("FX $_"); Start-Sleep 5; $p.ReadExisting() }
$p.WriteLine("CALL_OFF"); Start-Sleep 1; $p.Close()
```

**Por HTTP (requiere `secrets.h`):**

```powershell
$ip = "192.168.1.50"   # la IP que muestra pio device monitor
Invoke-RestMethod -Method Post -Uri "http://$ip/led" -ContentType text/plain -Body "CALL_ON"    # -> OK
Invoke-RestMethod -Method Post -Uri "http://$ip/led" -ContentType text/plain -Body "CALL_OFF"
```

Con un cuerpo inválido responde HTTP 400.

**De punta a punta con la app:**

```powershell
.\teams_monitor.exe --serial COM16     # o --http http://$ip/led
```

Entra a una llamada de Teams: en unos 2 s el anillo se enciende y aparece la notificación. Al colgar se apaga. Log en `%LOCALAPPDATA%\teams_monitor\teams_monitor.log`.
