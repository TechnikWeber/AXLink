# Feature request to F4HWN / F4WAT (KISS TNC)

Status: posted on 2026-11-02 as issue #763; Armel converted it into
[Discussion #764](https://github.com/armel/uv-k1-k5v3-firmware-custom/discussions/764) (Ideas).
The text below is the draft as posted.

Notes:

- Target: <https://github.com/armel/uv-k1-k5v3-firmware-custom>, a feature request, **not a bug
  report**. The behaviour is documented in the app's README (*Known limits*) and in `app_api.h`,
  and it suits APRSdroid.
- The KISS app is by Johan Denoyer F4WAT (GitHub `jdenoy`).

````markdown
**Title:** KISS TNC: optional ACKMODE (0x0C) and USB flow control for hosts that send many frames

Hi Armel, hi Johan,

thank you for the KISS TNC app. We would like to use it as the modem for a PC host that
carries IP over AX.25 (UI frames, PID 0xCC, up to 256 info bytes). For that kind of host the
app's documented limits become the bottleneck. This is not a bug report: for APRSdroid, which
sends one frame per beacon, everything works as the README describes. It is a request for small,
opt-in additions for hosts that send more than one frame.

What we read in the source (v6.3.0, the same on `feature_update_v7`):

1. **One frame at a time.** While a host frame waits for the channel, `kissByte()` drops
   whatever follows (`if(!g.kIn) return;`). That frame is not counted in `drop N`, and the host
   cannot tell when the pending frame has gone out.
2. **USB intake.** `usbd_cdc_acm_bulk_out()` re-arms the OUT endpoint at once, the ring
   (`VCP_RxBuf`) holds 256 bytes, and `kissPoll()` reads 16 bytes every 10 ms. When 256 bytes
   or more arrive before the next read, `app_serial_read()` flushes the ring
   (`APP_SERIAL_LOST`). A KISS frame with a 256-byte payload is about 275 bytes, so a host that
   writes it in one go loses it. (The API comment says to poll before 256 bytes can arrive, so
   this is by design.)

Proposals, all backward compatible:

- **A. ACKMODE** (G8BPQ's KISS extension, command 0x0C), opt-in by the host. The host sends
  `C0 0C hi lo <AX.25 frame> C0`; the app keeps the 2-byte tag with the pending frame and,
  once `transmit()` has finished, sends `C0 0C hi lo C0`. A host that never sends 0x0C sees no
  change, so APRSdroid is not affected. NinoTNC supports it and LinBPQ uses it. The host can
  then send the next frame exactly when the previous one has left. Our estimate: a few dozen
  bytes of the ~470 bytes still free in the overlay.
- **B. Count** the host frames dropped because another one was pending (in `drop N` or as a
  separate counter), so a pacing problem shows on the screen.
- **C. Optional, resident: flow control on the CDC OUT endpoint.** Re-arm it only when the
  ring has room for a full transfer (`read_buffer` is 128 bytes), and re-arm it from
  `serial_read()` once that room is free. Host writes then block instead of losing data, for
  every app that uses the serial API.
- **D. Optional: identification and RSSI.** Answer a query (for example KISS SetHardware 0x06)
  with name, version, maximum frame length and ACKMODE support, so a host can choose its
  settings; and report the RSSI with each received frame (the app already reads it for the
  screen). If the DCD-over-KISS extension that M0LTE and the NinoTNC designer are discussing
  settles, we would follow that format rather than invent our own.

In the meantime our host paces its writes (at most 64 bytes, about 40 ms apart, one frame in
flight), which works with the current app. We are prototyping A and B in our fork
(https://github.com/TechnikWeber/uv-k1-k5v3-aprs-firmware) and would be glad to send a pull
request if you are interested, or to keep it in the fork if you prefer the app as it is.

Thanks for the firmware and the app platform!
````
