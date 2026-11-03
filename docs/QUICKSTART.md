# AXLink quick start

From the box to the first ping, on Linux, Windows or macOS. AXLink is alpha: it works in the lab
and its network adapter passes its tests on all three systems, but it has **not been on the air
with a real radio yet**. Try it on a bench first, with a dummy load or on a free frequency.

## What you need

- **A radio with KISS**, for example a Quansheng UV-K1 or UV-K5 V3 with the
  [F4HWN firmware](https://github.com/armel/uv-k1-k5v3-firmware-custom) Labs v6.3.0 and its
  **KISS TNC** app, or our variant [AXLink TNC](https://github.com/TechnikWeber/AXLink-TNC)
  (adds ACKMODE). Any other KISS TNC works too: Dire Wolf with a sound card, a hardware TNC,
  KISS over TCP.
- **A USB-C cable and a clip-on ferrite** (VHF material, type 31 or 43) at the radio end, the
  cable looped 2-4 turns through it. Without it, the RF at key-up can reset the radio's USB port.
- **A computer**: Linux, Windows 10/11 or macOS, with administrator rights (the network adapter
  needs them).
- **Your own callsign.** AXLink refuses placeholders such as `N0CALL`. Every operator is
  responsible for what is allowed on their frequency; AXLink restricts nothing.
- **A frequency and an address plan** agreed with the other stations: each station gets its own
  address in the same subnet, e.g. `10.73.0.1/24`, `10.73.0.2/24`, … There is no DHCP on the
  radio. Do not use the APRS frequency for IP traffic.

## 1. The radio

1. Put F4HWN Labs v6.3.0 on the radio with [UV Studio](https://armel.github.io/uvstudio/).
2. Install the KISS TNC app, or AXLink TNC: download the zip from its
   [releases](https://github.com/TechnikWeber/AXLink-TNC/releases), unzip it, then UV Studio →
   Apps → Community → Install a .app → `AXLinkTNC.app`.
3. Set the VFO to the agreed frequency, FM. Start the app: `F` + `7`, select it, `MENU`.
4. Connect the radio to the computer with the USB-C cable (ferrite at the radio end).

## 2. AXLink

Download the package for your system from the
[releases](https://github.com/TechnikWeber/AXLink/releases) and unpack it. Each package holds
`axlinkd`, this guide, the README and the licences. Replace `<CALL>` with your callsign and
`10.73.0.1/24` with your address.

### Linux

```sh
tar xzf axlinkd-0.2.0-alpha.1-linux-amd64.tar.gz      # arm64: 64-bit Raspberry Pi; armv6: any Raspberry Pi
cd axlinkd-0.2.0-alpha.1-linux-amd64
./axlinkd ports                                         # the radio is marked "radio with F4HWN firmware"
sudo ./axlinkd run -call <CALL>-1 -port auto -ip 10.73.0.1/24
```

- `-port auto` finds the F4HWN radio by its USB ID (`36B7:FFFF`); otherwise `-port /dev/ttyACM0`.
- The adapter is called `axl0`. Without `sudo`, the user needs `CAP_NET_ADMIN` and access to
  the serial port (group `dialout`); that variant is not tested yet.

### Windows

1. Unpack the zip (`windows-amd64`; `windows-arm64` for ARM PCs), e.g. to `C:\AXLink`. It
   contains `wintun.dll`, the driver library of the network adapter, with its licence.
2. Open PowerShell **as administrator** (right-click → Run as administrator):

```powershell
cd C:\AXLink
.\axlinkd.exe ports                                     # the radio is a COM port, e.g. COM5
.\axlinkd.exe run -call <CALL>-1 -port auto -ip 10.73.0.1/24
```

- SmartScreen may warn about an unknown publisher: the program is not signed.
- The adapter is called `axl0`. Windows treats it as a public network, so its own firewall
  blocks incoming connections there (pings to this PC, servers on it). Outgoing connections and
  their answers pass. To allow pings, for example:
  `New-NetFirewallRule -DisplayName "AXLink ping" -Direction Inbound -InterfaceAlias axl0 -Protocol ICMPv4 -IcmpType 8 -Action Allow`
  (not tested yet).

### macOS

```sh
tar xzf axlinkd-0.2.0-alpha.1-macos-arm64.tar.gz       # Apple silicon; Intel Macs: macos-amd64
cd axlinkd-0.2.0-alpha.1-macos-arm64
xattr -d com.apple.quarantine axlinkd                   # the program is not signed
./axlinkd ports                                         # the radio is /dev/cu.usbmodem...
sudo ./axlinkd run -call <CALL>-1 -port auto -ip 10.73.0.1/24
```

- macOS names the adapter itself (`utun4`, `utun5`, …); the log and the web interface show it.

### From the source code (all systems)

```sh
git clone https://github.com/TechnikWeber/AXLink.git && cd AXLink
bash scripts/setup-go.sh && export PATH="$PWD/.tools/go/bin:$PATH"   # or a Go of the version in go.mod
go build -o axlinkd ./cmd/axlinkd
```

On Windows without the package, `scripts/fetch-wintun.ps1` puts `wintun.dll` next to
`axlinkd.exe` (see [DEVELOPMENT.md](DEVELOPMENT.md)).

## 3. The web interface

Open **http://127.0.0.1:7373** in a browser on the same computer (English or German, as the
browser prefers). Stop AXLink with `Ctrl+C` in its window.

- **Overview:** the live monitor (channel load, transmit queue, throughput), this station, the
  stations heard with their roles, locator and distance, the internet gateway, the firewall and
  the counters. Other AXLink stations appear by their beacon within seconds of their start, or
  at their next beacon (every 10 minutes).
- **Chat:** the room `CQ` for everyone and one conversation per station. Messages go as APRS
  messages, so APRS messenger radios can join; a message waits in the mailbox until its station
  is heard again.
- **Monitor:** every frame on the channel, those for other stations too, with a short
  description; for debugging.
- **Settings:** the welcome page (who you are, locator), sharing your internet connection, the
  firewall, port forwarding and the relay (on by default). They are stored and take effect at
  once.

Then try, from another station's computer: `ping 10.73.0.1`, or open `http://10.73.0.1/`, the
welcome page. At 1200 bit/s a ping takes about two seconds there and back.

## 4. What you can switch on

| You want | Web interface | or at the start |
|---|---|---|
| share your internet connection | Settings → Share the internet | `-proxy 8080` |
| use another station's internet | Overview → Use (then set `127.0.0.1:7374` as the proxy in the browser) | `-gateway auto` |
| open a service on this computer | Settings → Firewall → Add a port | `-open 8080/tcp` |
| … only for some stations | the same, "Only for" | `-open 22/tcp=<CALL>,<CALL>` |
| publish a service of your network | Settings → Port forwarding | `-forward 2222=192.168.1.10:22=<CALL>` |
| relay for stations out of range | on by default; Settings → Relay | `-relay=false` turns it off |
| see every frame on the channel | the view Monitor | `-pcap FILE` (Wireshark), `-debug` (log) |
| announce your locator | Settings → Welcome page | — |
| our TNC app with ACKMODE | — | `-profile f4hwn-kisstnc-ackmode` |
| a station that runs unattended | — | `-tags AUTO` |

Options given at the start are stored like a change in the web interface. All options:
`axlinkd run -h`.

## 5. When something does not work

- **"TNC not connected"** in the web interface: is the app running on the radio, the cable
  plugged in, the port right (`axlinkd ports`)? AXLink reconnects by itself after a USB reset.
- **No stations heard:** the same frequency and mode? Beacons go out at the start and every 10
  minutes; the counters show beacons sent and heard.
- **The link drops when transmitting:** RF on the USB cable. Ferrite at the radio end, antenna
  away from the cable, a short cable.
- **A station is refused:** this station's firewall lets in only answers, pings and opened
  ports. The other side gets "connection refused" at once; the overview lists the last refusals.
- **Slow:** 1200 bit/s is about 100 bytes per second for everyone on the frequency together. A
  web page of a few kilobytes takes tens of seconds; HTTPS needs a minute or two to set up.

## Without a radio: the lab (Linux)

```sh
bash scripts/lab-netns.sh     # two stations through a simulated channel
bash scripts/lab-mesh.sh      # three stations in a line, the middle one relays
```

Network namespaces, no root, no Docker; details in [DEVELOPMENT.md](DEVELOPMENT.md).
