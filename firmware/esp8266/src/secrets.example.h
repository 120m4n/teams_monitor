// Copy to secrets.h (gitignored) to enable HTTP control. Without it the board is serial-only.
// deploy/flash.ps1 writes secrets.h from teams_monitor.xml.
#define WIFI_SSID "your-2.4GHz-ssid"
#define WIFI_PASS "your-password"
// Optional static IP (all three or none); without them the board uses DHCP.
// #define WIFI_IP "192.168.1.50"
// #define WIFI_GATEWAY "192.168.1.1"
// #define WIFI_SUBNET "255.255.255.0"
