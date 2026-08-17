# Getting Started with Nymeria

Nymeria is a web-based APRS client. You run it on a computer, then open a browser to use it. It works on Windows, Linux, Mac, and Raspberry Pi. The whole thing is a single file you can download and run — no installation required.

This guide walks through everything from zero to stations-on-the-map. Every step matters, and they go in order. If something isn't working, come back here and make sure you didn't skip anything.

---

## Table of Contents

1. [What You Need](#what-you-need)
2. [Download Nymeria](#download-nymeria)
3. [First Launch](#first-launch)
4. [The Setup Wizard](#the-setup-wizard)
5. [Logging In](#logging-in)
6. [Using Nymeria](#using-nymeria)
7. [Multi-User Access](#multi-user-access)
8. [Adding RF with Direwolf](#adding-rf-with-direwolf)
9. [Running Nymeria as a Service](#running-nymeria-as-a-service)
10. [Troubleshooting](#troubleshooting)

---

## What You Need

Before you start, here's the full list.

**Required:**
- A computer (Windows 10+, Linux, macOS, or Raspberry Pi OS)
- An internet connection
- Your amateur radio callsign (you need a license to transmit — receive-only works without one)
- About 5 minutes

**Optional (for RF):**
- A TNC, or a radio with a soundcard interface and [Direwolf](https://github.com/wb2osz/direwolf)
- A USB sound device if your radio doesn't have a built-in interface

---

## Download Nymeria

Go to the [Nymeria releases page](https://github.com/longjos/Nymeria/releases) and download the latest binary for your platform. Binaries are named `nymeria-VERSION-PLATFORM` (e.g., `nymeria-v0.2.0-linux-amd64`).

| Platform | Look for |
|----------|----------|
| Windows (64-bit) | `nymeria-vX.X.X-windows-amd64.exe` |
| Linux (64-bit) | `nymeria-vX.X.X-linux-amd64` |

More platforms (Linux ARM, macOS) will be added in future releases.

**Windows:** Move the `.exe` to a folder you'll remember (e.g., `C:\Nymeria\`). No installer needed.

**Linux:** Move the binary to wherever you like, then make it executable:

```bash
chmod +x nymeria-*-linux-amd64
```

You can rename it to just `nymeria` for convenience:

```bash
mv nymeria-*-linux-amd64 nymeria
```

---

## First Launch

Open a terminal (Command Prompt or PowerShell on Windows) and run:

**Linux / macOS:**
```bash
./nymeria
```

**Windows:**
```
nymeria.exe
```

You should see log output that says Nymeria is listening. By default it runs on port 8080. Open your browser and go to:

**http://localhost:8080**

If port 8080 is already in use on your machine, use a different one:

```bash
./nymeria --listen :9090
```

Then open http://localhost:9090 instead.

**Accessing from another device on your network** (phone, tablet, another computer): use your machine's local IP address instead of `localhost`. For example, `http://192.168.1.42:8080`. You can find your IP with `ip addr` on Linux, `ipconfig` on Windows, or `ifconfig` on Mac.

---

## The Setup Wizard

The first time you open Nymeria in a browser, you'll see a setup wizard. It walks you through the essential configuration so you don't have to edit any files by hand.

### Step 1: Welcome

A brief intro. Click **Next** to begin.

### Step 2: Station Identity

Enter your callsign (letters and numbers, no punctuation) and pick an SSID. If you're not sure which SSID to use, leave it at 0. The SSID dropdown explains what each number conventionally means (0 = primary, 5 = smartphone, 9 = mobile, etc.).

You can also enter a status comment (up to 43 characters) — this is broadcast alongside your position.

### Step 3: Station Location

Click the map to drop a pin at your station's location, or type in coordinates directly. This sets two things: your APRS position report and the center of your APRS-IS filter (which controls what stations you see).

You can drag the pin to adjust. Zoom in for precision.

### Step 4: APRS-IS Connection

APRS-IS is the internet backbone of the APRS network. Connecting to it lets you see stations from around the world — no radio required.

Leave this enabled unless you're running a purely RF setup. The defaults (server: `rotate.aprs2.net`, port: `14580`) are correct for almost everyone. The filter and passcode are computed automatically from your location and callsign.

### Step 5: Review and Save

Double-check everything, then hit **Save & Launch**. Nymeria writes your settings to `nymeria.yaml` and connects immediately. No restart needed.

Within a few seconds, stations should start appearing on the map.

---

## Logging In

After the setup wizard completes, you'll see a login screen. Enter a display name (your callsign works well) and click **Request Access**.

**First user**: You're automatically approved as an **Admin** with full access. No waiting, no approval needed. The app opens immediately.

**Additional users**: When other people connect to your Nymeria instance, they enter their name and request access. They'll see a "Waiting for admin approval" screen until an admin approves them. See [Multi-User Access](#multi-user-access) below.

Your session is saved in the browser. If you close the tab and come back, you'll be logged in automatically — no need to re-enter your name.

---

## Using Nymeria

Once connected, you'll see the map with APRS stations plotted in real time. Here's a quick orientation:

### The Map

- **Click a station** on the map to see its details — position, path, weather data (if it's a weather station), telemetry, and more
- **Station icons** reflect the APRS symbol the station is broadcasting (car, house, weather station, etc.)
- **Tracks** show where mobile stations have been — toggle these on/off in the toolbar
- On desktop, station details appear in a **side panel**. On mobile, they slide up from the bottom

### Toolbar

The toolbar across the top gives you access to everything:

- **Messages** — Send and receive APRS messages with delivery confirmation. Messages automatically retry until acknowledged
- **Stations** — Browse all heard stations in a sortable list
- **Settings** — Transport configuration, beaconing, display preferences
- **Annotations** — Drop pins, draw lines and areas on the map for situational awareness. Annotations support 14 categories (shelter, hazard, route, boundary, etc.) with color and style customization
- **Net Control** — Manage APRS nets with roster, missions, timeline, and roll call (see below)

### Key Features

- **Position beaconing** — Enable in settings to periodically broadcast your position to the APRS network
- **Objects** — Create APRS objects (event markers, assets, etc.) that other stations can see on the network
- **Weather** — Weather stations are automatically detected and displayed with temperature, wind, pressure, and humidity charts
- **Telemetry** — Stations broadcasting telemetry show graphs with parameter names, units, and scaling equations
- **Packet inspector** — View raw APRS packets for debugging and learning
- **Filter builder** — Visual editor for APRS-IS server-side filters (range, prefix, friend, type, etc.)

### Net Control

For organized APRS operations (emergency communications, public service events, etc.), the Net Control dashboard provides:

- **Nets** — Create and manage operational nets with NCS (Net Control Station) designation
- **Roster** — Check in stations, assign tactical callsigns, track status
- **Missions** — Task tracking and assignment within a net
- **Timeline** — Chronological log of net activity
- **Roll call** — One-click check-in prompts for all roster stations
- **Command palette** — Keyboard-driven interface for high-tempo NCS operations (press `/` to open)
- **Agency dashboard** — Multi-net overview for EOC/ICP display, showing station counts, mission status, and map overlays per net

### Annotations

Annotations are Nymeria's geographic markup system. Use them to mark locations, draw routes, define boundaries, and highlight areas of interest:

- **14 categories**: Shelter, staging, hazard, route, boundary, no-go zone, checkpoint, and more
- **Net scoping**: Link annotations to a specific net so they appear in that net's dashboard
- **Import/Export**: Import GPX and KML files; export as GeoJSON
- **APRS bridge**: Publish annotations as APRS objects so they're visible to other stations on the network
- **Click to edit**: Click any annotation on the map to jump to it in the panel for quick editing

---

## Multi-User Access

Nymeria supports multiple simultaneous users with role-based access. This is designed for field deployments where multiple operators share one Nymeria instance.

### How It Works

1. The **first person** to connect becomes the **Admin** automatically — no configuration needed
2. When additional people connect, they enter a display name and **request access**
3. They see a "Waiting for admin approval" screen
4. The admin sees pending requests and can **approve** (assigning a role) or **deny** each one
5. Approved users are notified instantly and can start using Nymeria — no page refresh needed

### Roles

| Role | What they can do |
|------|------------------|
| **Admin** | Everything, plus approve/deny users and manage settings |
| **Operator** | Send messages, transmit beacons/objects, manage nets and annotations |
| **Plotter** | Create and manage map annotations, tactical aliases, and APRS objects |
| **Observer** | View the map, stations, messages, and dashboards (read-only) |

Admins can promote or demote users after approval.

### Session Persistence

- Sessions survive browser refreshes — your token is saved locally
- If you're inactive for 30 minutes, your session expires
- Reconnecting within 4 hours restores your previous role automatically (no re-approval needed)
- After 4 hours, you'll need to request access again

---

## Adding RF with Direwolf

APRS-IS is great for internet-connected monitoring, but the real magic of APRS happens over RF. [Direwolf](https://github.com/wb2osz/direwolf) is a software TNC (Terminal Node Controller) that turns your computer's sound card into a radio modem.

### What You Need for RF

- A VHF radio (most APRS activity is on 144.390 MHz in North America, 144.800 MHz in Europe)
- A way to connect the radio to your computer (a soundcard interface like a SignaLink, Digirig, or a radio with built-in USB audio)
- Direwolf installed on the same machine (or accessible over your network)

### Installing Direwolf

**Linux / Raspberry Pi:**
```bash
sudo apt install -y direwolf
```

**Windows / Mac:** Download from the [Direwolf releases page](https://github.com/wb2osz/direwolf/releases).

### Configuring Direwolf

Direwolf needs a config file. Create one at `direwolf.conf`:

```
# Your callsign
MYCALL N0CALL

# Sound device — find yours with "arecord -l" on Linux or check Direwolf docs
ADEVICE plughw:1,0

# KISS TCP server — this is how Nymeria connects
KISSPORT 8001

# Modem settings for 1200 baud APRS
MODEM 1200

# Receive on channel 0
CHANNEL 0
```

Replace `N0CALL` with your callsign and `plughw:1,0` with your actual sound device. Finding the right audio device name is the hardest part of this whole process — the Direwolf documentation covers it in detail for each platform.

Start Direwolf:
```bash
direwolf -t 0 -c direwolf.conf
```

You should see Direwolf start decoding packets if your radio is tuned to an active APRS frequency.

### Connecting Nymeria to Direwolf

Edit your `nymeria.yaml` and add a KISS TCP transport:

```yaml
transports:
  # Keep your APRS-IS connection (if you want both)
  - type: aprsis
    host: rotate.aprs2.net
    port: 14580
    filter: "r/35.0/-106.0/200"

  # Add Direwolf
  - type: kisstcp
    host: localhost
    port: 8001
```

Restart Nymeria (Ctrl+C, then `./nymeria` again). The transport status indicator in the toolbar will show both connections.

With dual transports, Nymeria automatically deduplicates packets that arrive from both APRS-IS and RF, so you won't see doubles.

### Docker Compose (Direwolf + Nymeria Together)

If you prefer containers, the repo includes a Docker Compose setup that runs both Direwolf and Nymeria together:

```bash
# Copy the example configs
cp docker/direwolf.conf.example docker/direwolf.conf
cp docker/nymeria.yaml.example docker/nymeria.yaml

# Edit both files with your callsign, audio device, and location

# Start everything
docker compose up -d
```

This is especially handy on a Raspberry Pi where you want Nymeria to start automatically on boot.

### Hardware TNC (Serial KISS)

If you have a dedicated hardware TNC (Mobilinkd, TNC-X, KPC-3+) or a Kenwood TH-D74/D75 in KISS, you can skip Direwolf. The easiest path is **Settings → Transports → Add Serial**: Nymeria lists COM/`tty` ports on the server and fills baud from a device profile.

For a TH-D74/D75: press **[F][LIST]** until the display shows **KISS+1200** or **KISS+9600**. On Windows, install Kenwood’s USB CDC VCP driver *before* plugging in (not Silicon Labs CP210x). Menu 505 is the on-air rate, not USB baud.

```yaml
transports:
  - type: serial
    device: COM5             # or /dev/serial/by-id/… on Linux
    baud: 9600               # USB CDC usually ignores this

  # Linux fallback if you prefer to edit YAML:
  # - type: serial
  #   device: /dev/ttyACM0
  #   baud: 9600
```

---

## Running Nymeria as a Service

Once you have Nymeria working, you'll probably want it to start automatically.

### Linux (systemd)

Create `/etc/systemd/system/nymeria.service`:

```ini
[Unit]
Description=Nymeria APRS Client
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=pi
WorkingDirectory=/home/pi
ExecStart=/home/pi/nymeria --config /home/pi/nymeria.yaml
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Adjust the `User` and paths to match your setup, then:

```bash
sudo systemctl daemon-reload
sudo systemctl enable nymeria
sudo systemctl start nymeria
```

Check that it's running:
```bash
sudo systemctl status nymeria
```

### Windows

The simplest approach is to put a shortcut to `nymeria.exe` in your Startup folder:

1. Press `Win+R`, type `shell:startup`, press Enter
2. Create a shortcut to `nymeria.exe` in that folder
3. Right-click the shortcut > Properties > set "Start in" to the folder containing your `nymeria.yaml`

For a proper Windows service, look into [NSSM](https://nssm.cc/) (the Non-Sucking Service Manager).

### Docker

The Docker Compose setup handles restarts automatically (`restart: unless-stopped`). To start it on boot, just make sure Docker itself starts on boot (it does by default on most systems).

---

## Troubleshooting

### "Nothing happens when I open the browser"

Make sure you're using the right port. Check the terminal output — Nymeria prints the address it's listening on. If you used `--listen :9090`, you need to go to `http://localhost:9090`, not 8080.

### "No stations appear on the map"

- Did you complete the setup wizard? If the callsign is still `N0CALL`, the wizard should appear automatically.
- Check the transport status in the toolbar. If APRS-IS shows "disconnected," your internet connection may be blocked or the server may be unreachable.
- If you just connected, give it 30 seconds. APRS-IS delivers packets as they arrive — it's not instant.
- Check your filter. A filter like `r/35.0/-106.0/200` means "stations within 200 km of this point." If your coordinates are wrong, you might be filtering for an empty area.

### "I can see stations but can't send messages"

- You need a valid amateur radio callsign and a working transmit path (either APRS-IS with a valid passcode, or an RF connection through a TNC).
- The passcode is computed automatically from your callsign. If you manually entered the wrong one, update it in `nymeria.yaml` or re-run the setup wizard by resetting your callsign to `N0CALL` in the config file.
- Make sure your role is **Operator** or **Admin**. Observers have read-only access.

### "Other users can't connect"

- Make sure they're using your machine's IP address, not `localhost`. `localhost` only works on the machine running Nymeria.
- Check your firewall — port 8080 (or whatever you configured) needs to be open for incoming connections.
- New users will see "Waiting for admin approval" until an admin approves them. Check the pending requests in the admin panel.

### "I was approved but now I'm locked out"

- Sessions expire after 30 minutes of inactivity. Reconnecting within 4 hours restores your session automatically.
- If more than 4 hours have passed, you'll need to request access again.
- If the Nymeria server was restarted, all sessions are cleared. The first person to connect becomes admin again.

### "Direwolf isn't connecting"

- Is Direwolf actually running? Check its terminal output.
- Is it listening on port 8001? Run `ss -tlnp | grep 8001` on Linux or `netstat -an | findstr 8001` on Windows.
- If Nymeria and Direwolf are on different machines, change `host: localhost` to the IP address of the Direwolf machine. Make sure there's no firewall blocking port 8001.

### "It was working yesterday and now it's not"

- Check if your config file (`nymeria.yaml`) still exists and hasn't been corrupted.
- Check disk space: `df -h` on Linux. SQLite needs room to write.
- Check the logs. Nymeria prints to stdout — if you're running it as a service, use `journalctl -u nymeria -f` on Linux.

---

## Configuration Reference

Nymeria stores its configuration in `nymeria.yaml`, created automatically by the setup wizard. You can also edit it by hand. Here are the key sections:

```yaml
server:
  listen: ":8080"              # Address and port to listen on

station:
  callsign: "N0CALL"          # Your amateur radio callsign
  ssid: 0                     # SSID (0-15)
  lat: 35.0                   # Station latitude
  lon: -106.0                 # Station longitude
  stale_timeout: 80m          # Remove stations after this duration

transports:
  - type: aprsis               # APRS-IS internet gateway
    host: rotate.aprs2.net
    port: 14580
    filter: "r/35.0/-106.0/200"

store:
  path: "./nymeria.db"         # SQLite database location

beacon:
  enabled: false               # Position beaconing
  interval: 10m                # Beacon interval

logging:
  level: "info"                # debug, info, warn, error
```

### CLI Flags

```
./nymeria [flags]

  --config PATH    Config file path (default: nymeria.yaml)
  --listen ADDR    Override listen address (e.g., :9090)
  --version        Print version and exit
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `NYMERIA_LISTEN` | Server listen address |
| `NYMERIA_CALLSIGN` | Station callsign |
| `NYMERIA_DB_PATH` | SQLite database path |
| `NYMERIA_LOG_LEVEL` | Log level |

---

## Next Steps

- **Net control**: Set up operational nets for emergency communications or public service events
- **Annotations**: Mark locations, draw routes, and define boundaries for situational awareness
- **Checkpoint tracking**: Track event progress with checkpoints along a route
- **Agency dashboard**: Monitor multiple nets from a single overview screen
- **Weather and telemetry**: Weather stations and telemetry sources are automatically detected and displayed with charts
- **Offline tiles**: Cache map tiles for use in areas without internet

For the full feature list, see the [README](README.md). For building from source, see the [Development section](README.md#development) of the README.
