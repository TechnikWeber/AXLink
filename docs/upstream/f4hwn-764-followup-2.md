# Draft: second follow-up in Discussion #764

For [Discussion #764](https://github.com/armel/uv-k1-k5v3-firmware-custom/discussions/764).
The owner posts it; check the facts below against the repositories first. Post it as is, or
wait for the bench test and add its results.

---

@jdenoy, @armel: the first three points of this request are now built in AXLink TNC 0.2, each
as a separate commit on top of the unmodified KISS TNC 0.9 from v6.3.0:

| Commit | Change |
|---|---|
| [4a25af6](https://github.com/TechnikWeber/AXLink-TNC/commit/4a25af6) | **ACKMODE** (`0x0C`): the host sends `C0 0C hi lo <frame> C0`, the app answers `C0 0C hi lo C0` once the frame has left the TNC (sent, or dropped in RX-only mode). A host that never sends `0x0C` sees no change. |
| [ce3a675](https://github.com/TechnikWeber/AXLink-TNC/commit/ce3a675) | A host frame that arrives while another one waits for the channel is counted in `lost N` (line 3). A resync (`FESC 00 FEND`) is not counted. |
| [b9537d4](https://github.com/TechnikWeber/AXLink-TNC/commit/b9537d4) | A host frame cut off by a USB re-enumeration (`APP_SERIAL_ENUM`) or a lapped ring (`APP_SERIAL_LOST`) is dropped; the parser waits for the next FEND instead of sending the cut frame. |

Each commit adds its checks to `test/test_tnc.py` (28 checks in all). They use the drawing log
that [b8c3573](https://github.com/TechnikWeber/AXLink-TNC/commit/b8c3573) adds to
`test/host_tnc.c`. To take all four into the firmware tree:

```sh
git -C AXLink-TNC format-patch --relative=app -1 b8c3573 --start-number 1 -o /tmp/p
git -C AXLink-TNC format-patch --relative=app 4a25af6~1..b9537d4 --start-number 2 -o /tmp/p
git am --directory=App/apps/kisstnc /tmp/p/*.patch
```

I tried exactly that on v6.3.0: the patches apply and `test/test_tnc.py` passes. Size:
3,812 B of the 4 KiB overlay (ARM GNU Toolchain 13.3.rel1). **Not yet run on a radio**; a
bench test with two UV-K1 is next, and I will report it here.

AXLink uses ACKMODE with this app, and keeps pacing its writes with the original app. USB flow
control in the resident firmware is still the part only you can do.
