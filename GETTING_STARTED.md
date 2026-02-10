# Getting Started with Nymeria

Nymeria is a web-based APRS client. You run it on a computer, then open a browser to use it. It works on Windows, Linux, Mac, Raspberry Pi — anything that runs Go. The whole thing compiles down to a single file you can copy anywhere.

This guide walks through everything from zero to stations-on-the-map. Every step matters, and they go in order. If something isn't working, come back here and make sure you didn't skip anything.

---

## Table of Contents

1. [What You Need](#what-you-need)
2. [Install the Prerequisites](#install-the-prerequisites)
3. [Build Nymeria](#build-nymeria)
4. [First Launch](#first-launch)
5. [The Setup Wizard](#the-setup-wizard)
6. [Using Nymeria](#using-nymeria)
7. [Adding RF with Direwolf](#adding-rf-with-direwolf)
8. [Running Nymeria as a Service](#running-nymeria-as-a-service)
9. [Troubleshooting](#troubleshooting)

---

## What You Need

Before you start, here's the full list. Gather all of this first.

**Required:**
- A computer (Windows 10+, Linux, macOS, or Raspberry Pi OS)
- An internet connection
- Your amateur radio callsign (you need a license to transmit — receive-only works without one)
- About 10 minutes

**Optional (for RF):**
- A TNC, or a radio with a soundcard interface and [Direwolf](https://github.com/wb2osz/direwolf)
- A USB sound device if your radio doesn't have a built-in interface

**Software you'll install:**
- [Go](https://go.dev/dl/) 1.24 or newer
- [Node.js](https://nodejs.org/) 22 or newer
- [pnpm](https://pnpm.io/) (Node package manager)
- [Git](https://git-scm.com/) (to download the source code)
- Make (Linux/Mac have it already; Windows needs a small install)

---

## Install the Prerequisites

Pick your platform below. Do every step — they're all necessary.

### Linux (Ubuntu / Debian / Raspberry Pi OS)

Open a terminal and run these commands one at a time:

```bash
# Update package lists
sudo apt update

# Install build tools and git
sudo apt install -y build-essential git

# Install Go (check https://go.dev/dl/ for the latest version)
# For x86_64:
wget https://go.dev/dl/go1.24.4.linux-amd64.tar.gz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.24.4.linux-amd64.tar.gz

# For Raspberry Pi (arm64):
# wget https://go.dev/dl/go1.24.4.linux-arm64.tar.gz
# sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.24.4.linux-arm64.tar.gz

# Add Go to your PATH (add this line to ~/.bashrc or ~/.zshrc too)
export PATH=$PATH:/usr/local/go/bin

# Install Node.js 22
curl -fsSL https://deb.nodesource.com/setup_22.x | sudo -E bash -
sudo apt install -y nodejs

# Install pnpm
npm install -g pnpm
```

**Verify everything installed** (do not skip this):

```bash
go version        # Should say go1.24 or higher
node --version    # Should say v22.x.x or higher
pnpm --version    # Should print a version number
make --version    # Should print GNU Make info
git --version     # Should print git version info
```

If any of those commands say "not found," stop and fix that one before continuing.

### Windows

1. **Install Git**: Download from https://git-scm.com/download/win and run the installer. Use the default options. This gives you "Git Bash," which you'll use as your terminal for the rest of this guide.

2. **Install Go**: Download the `.msi` installer from https://go.dev/dl/ and run it. Defaults are fine.

3. **Install Node.js**: Download the LTS installer from https://nodejs.org/ and run it. Check the box that says "Automatically install necessary tools" if it appears.

4. **Install pnpm**: Open Git Bash and run:
   ```bash
   npm install -g pnpm
   ```

5. **Install Make**: Still in Git Bash:
   ```bash
   # Option A: If you have choco (Chocolatey package manager)
   choco install make

   # Option B: If you don't have choco, install it first:
   # https://chocolatey.org/install
   # Then run: choco install make
   ```

**Verify everything** in Git Bash:

```bash
go version
node --version
pnpm --version
make --version
git --version
```

All five commands should print version information. No errors, no "not found." If one fails, fix it before moving on.

### macOS

```bash
# Install Xcode command line tools (includes Make and Git)
xcode-select --install

# Install Homebrew if you don't have it
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

# Install Go and Node
brew install go node

# Install pnpm
npm install -g pnpm
```

**Verify:**

```bash
go version
node --version
pnpm --version
make --version
git --version
```

---

## Build Nymeria

This is the same on every platform. Open your terminal (Git Bash on Windows) and run:

```bash
# Download the source code
git clone https://github.com/longjos/Nymeria.git
cd Nymeria

# Build everything
make build
```

The build takes a minute or two. When it finishes, you'll have a `nymeria` binary (or `nymeria.exe` on Windows) in the current directory.

**If the build fails**, the most common causes are:
- You skipped a prerequisite. Go back and run the verification commands.
- pnpm shows a warning about `approve-builds` for esbuild. This is harmless — run `cd web && pnpm approve-builds && cd ..` then try `make build` again.
- On Windows, if `make` isn't found, you need to install it (see the Windows prerequisites section).

**Cross-compiling for Windows** (from a Linux or Mac machine):

```bash
make windows
```

This produces `nymeria.exe` that you can copy to any Windows machine. No install needed on the target — it's a single file.

---

## First Launch

```bash
# Copy the example config
cp nymeria.example.yaml nymeria.yaml

# Start Nymeria
./nymeria
```

On Windows:
```bash
copy nymeria.example.yaml nymeria.yaml
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

The first time you open Nymeria in a browser, you'll see a setup wizard. It has four steps:

### Step 1: Station Identity

Enter your callsign (letters and numbers, no punctuation) and pick an SSID. If you're not sure which SSID to use, leave it at 0. The SSID dropdown explains what each number conventionally means.

### Step 2: Station Location

Click the map to drop a pin at your station's location, or type in coordinates directly. This sets two things: your APRS position report and the center of your APRS-IS filter (which controls what stations you see).

You can drag the pin to adjust. Zoom in for precision.

### Step 3: APRS-IS Connection

APRS-IS is the internet backbone of the APRS network. Connecting to it lets you see stations from around the world — no radio required.

Leave this enabled unless you're running a purely RF setup. The defaults (server: `rotate.aprs2.net`, port: `14580`) are correct for almost everyone. The filter and passcode are computed automatically from your location and callsign.

### Step 4: Review and Save

Double-check everything, then hit **Save & Launch**. Nymeria writes your settings to `nymeria.yaml` and connects immediately. No restart needed.

Within a few seconds, stations should start appearing on the map.

---

## Using Nymeria

Once connected, you'll see the map with APRS stations plotted in real time. Here's a quick orientation:

- **Click a station** on the map to see its details — position, path, weather data (if it's a weather station), telemetry, etc.
- **Messages** — Send and receive APRS messages with delivery confirmation. Messages automatically retry until acknowledged.
- **Beaconing** — If enabled in settings, Nymeria will periodically transmit your position to the network.
- **Objects** — Create APRS objects (event markers, assets, etc.) that other stations can see.
- **Settings** — Accessible from the toolbar. You can change your transport configuration, enable/disable features, and manage multi-user access.

The interface works on desktop and mobile. On desktop, station details appear in a side panel. On mobile, they slide up from the bottom.

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

If you have a dedicated hardware TNC (like a TNC-X, Mobilinkd, or KPC-3+) connected via USB or serial port, you can skip Direwolf entirely:

```yaml
transports:
  - type: serial
    device: /dev/ttyUSB0    # Linux — check "ls /dev/ttyUSB*"
    baud: 9600               # Match your TNC's baud rate

  # On Windows, use the COM port:
  # - type: serial
  #   device: COM3
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
WorkingDirectory=/home/pi/Nymeria
ExecStart=/home/pi/Nymeria/nymeria --config /home/pi/Nymeria/nymeria.yaml
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

### "It built but nothing happens when I open the browser"

Make sure you're using the right port. Check the terminal output — Nymeria prints the address it's listening on. If you used `--listen :9090`, you need to go to `http://localhost:9090`, not 8080.

### "No stations appear on the map"

- Did you complete the setup wizard? If the callsign is still `N0CALL`, the wizard should appear automatically.
- Check the transport status in the toolbar. If APRS-IS shows "disconnected," your internet connection may be blocked or the server may be unreachable.
- If you just connected, give it 30 seconds. APRS-IS delivers packets as they arrive — it's not instant.
- Check your filter. A filter like `r/35.0/-106.0/200` means "stations within 200 km of this point." If your coordinates are wrong, you might be filtering for an empty area.

### "I can see stations but can't send messages"

- You need a valid amateur radio callsign and a working transmit path (either APRS-IS with a valid passcode, or an RF connection through a TNC).
- The passcode is computed automatically from your callsign. If you manually entered the wrong one, update it in `nymeria.yaml` or re-run the setup wizard by resetting your callsign to `N0CALL` in the config file.

### "Direwolf isn't connecting"

- Is Direwolf actually running? Check its terminal output.
- Is it listening on port 8001? Run `ss -tlnp | grep 8001` on Linux or `netstat -an | findstr 8001` on Windows.
- If Nymeria and Direwolf are on different machines, change `host: localhost` to the IP address of the Direwolf machine. Make sure there's no firewall blocking port 8001.

### "Build failed"

- Read the error message. It almost always tells you what's missing.
- `go: command not found` — Go isn't installed or isn't in your PATH.
- `pnpm: command not found` — Run `npm install -g pnpm`.
- `make: command not found` — See the prerequisites section for your platform.
- Errors about node modules — Try `cd web && rm -rf node_modules && pnpm install && cd ..` then `make build` again.

### "It was working yesterday and now it's not"

- Check if your config file (`nymeria.yaml`) still exists and hasn't been corrupted. A backup is easy: `cp nymeria.yaml nymeria.yaml.bak` before making changes.
- Check disk space: `df -h` on Linux. SQLite needs room to write.
- Check the logs. Nymeria prints to stdout — if you're running it as a service, use `journalctl -u nymeria -f` on Linux.

---

## Next Steps

- **Multi-user access**: Set a PIN in `nymeria.yaml` under `session.pin` to require authentication. Users who enter the correct PIN get Operator access; others get read-only Observer access. Promote users to Admin from the settings panel.
- **Position beaconing**: Enable in settings to periodically broadcast your position to the APRS network.
- **Map annotations**: Drop pins, draw lines, and mark areas on the map for local situational awareness.
- **Weather and telemetry**: Weather stations and telemetry sources are automatically detected and displayed with charts.

For the full feature list and configuration reference, see the [README](README.md).
