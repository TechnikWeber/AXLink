# Messenger and KISS TNC via Multiboot (interim until F4HWN v7)

> **Interim solution.** This guide applies only while the messenger is based on F4HWN v6.3.
> Once the messenger has moved to v7 and carries the app platform in the same firmware, it is
> deleted. See [Clean-up after v7](#clean-up-after-v7).

## What for

The messenger (APRS in the background) and the KISS TNC app (the radio as a modem for AXLink,
APRSdroid and other KISS programs) do not fit into one firmware on v6.3. With Multiboot both
live on the radio; at power-on you choose which one runs.

| Multiboot slot | Firmware | For |
|---|---|---|
| `M` (Main) | messenger, `f4hwn.messenger.bin` | every day: voice and the APRS messenger |
| `1` | F4HWN Labs v6.3.0, `f4hwn.labs.v6.3.0.bin` | the KISS TNC app for AXLink, APRSdroid |

The steps follow the F4HWN wiki (pages "Multiboot and Multiconfig", "UV Studio", "Overlay apps",
"App KISS TNC"), as of 2026-11-02.

**TNC only, without the messenger:** no Multiboot is needed. Flash the original F4HWN Labs
v6.3.0 normally (UV Studio → *Flash Firmware*), then install the KISS TNC app as in step 4
below, or our own [AXLink TNC](https://github.com/TechnikWeber/AXLink-TNC) as a `.app` through
UV Studio → *Apps* → *Community* → *Install a .app*. From v7 the same works with the original
Fusion.

## What you need

- A UV-K1 or UV-K5 V3 (PY32F071).
- A USB-C data cable; for transmitting, a clip-on ferrite (VHF material) at the radio end.
- A desktop browser with Web Serial (Chrome, Edge, Brave, Opera, Firefox 151 or later) for
  [UV Studio](https://armel.github.io/uvstudio/).

## Set up once

1. **Back up the calibration.** UV Studio → *Dump Calibration*; name the file after the radio's
   serial number.
2. **Flash the messenger as Main.** Put the radio into flash mode (DFU), UV Studio → *Flash
   Firmware* → local file `f4hwn.messenger.bin` (from the fork's releases) → *Flash firmware*.
   On first start the radio shows `Init Main`: do not switch it off.
3. **Write Labs into slot 1.** Start the radio normally (not DFU), UV Studio → *Firmware Slots* →
   refresh the table → choose *F4HWN Labs v6.3.0* from the catalog (or `f4hwn.labs.v6.3.0.bin`
   as a file) → slot `1`, name e.g. `LABS` → *Write to slot* → confirm and wait until erasing,
   writing and the CRC check are done. The running messenger stays unchanged.
4. **Switch to Labs** (see below) and install the app there: UV Studio → *Apps* → *Catalog* (UV
   Studio picks the matching v6.3.0 catalog) → *KISS TNC* → *Install*. Our own app installs as a
   `.app` file instead (*Community* → *Install a .app*), later from the Community tab.
5. **Set the frequency in Labs.** Labs uses its own configuration `CFG 1`: VFO on the data
   frequency (AXLink) or on 144.800 MHz (APRSdroid), FM, power.

## Switching

- **To Labs:** radio off, hold `MENU`, switch on, release at the `F4HWN MULTIBOOT` screen. Choose
  slot `1` (UV-K1: LEFT/RIGHT, UV-K5: UP/DOWN), `MENU`, and `MENU` again at `Restore ...?`.
  **Do not switch off** during `Writing / Verify`; the radio restarts afterwards.
- **Start the KISS TNC:** in Labs `F` + `7`, choose *KISS TNC* (or *AXLink TNC*), `MENU`. Then
  start AXLink on the PC or connect APRSdroid. `EXIT` ends the app.
- **Back to the messenger:** as above, but choose `M`. The mailbox and the messenger settings
  stay; they live in their own flash area.

## AXLink with it (Linux)

```sh
./axlinkd ports      # shows the radio with USB 36B7:FFFF
sudo ./axlinkd run -call <CALL>-1 -port auto -ip 10.73.0.1/24 -route 10.73.0.2=<PEER>-1
```

The profile `f4hwn-kisstnc` is the default for serial ports. Details:
[DEVELOPMENT.md](DEVELOPMENT.md).

## Good to know

- Every switch writes the firmware into the radio again; it takes a few seconds.
- Every slot has its own configuration (channels, VFOs): Main `CFG M`, Labs `CFG 1`. `SetCfg`
  can share a bank; according to F4HWN at your own risk, back up first.
- Apps live in the external flash and survive a switch; only Labs can start them.
- The app version must match the firmware: Labs v6.3.0 → apps from the v6.3.0 catalog.
- According to the source the flash areas do not overlap: Multiboot slots 0x020000–0x0BFFFF,
  Labs v6.3 app slots 0x102000–0x161FFF, messenger 0x1BA000–0x1C9FFF.
- Restoring the complete external flash in UV Studio also overwrites the messenger data.
- **Not checked on hardware yet.** That is the first test once the UV-K1 radios arrive.

## Clean-up after v7

Once the messenger runs on F4HWN v7 and carries the app platform:

- [ ] delete this file;
- [ ] remove the Multiboot part of D6 in `DECISIONS.md` and mark D6 as done;
- [ ] remove the references in `README.md`, `DEVELOPMENT.md` and `TODO.md`;
- [ ] in the firmware fork, replace the Multiboot note in the messenger README with "KISS TNC in
      the same firmware";
- [ ] check AXLink TNC on v7 and rebuild it if v7 changes the ABI.
