# Upstream threads

Conversations with other projects, and what we owe them. When a change or a finding affects
something we told them, note it under "To tell them next time". Post an update once there is
something they can use: bench measurements, a commit they can take, or a correction. Drafts
live in this folder.

## F4HWN firmware and its KISS TNC app

Armel F4HWN (firmware, GitHub `armel`) and Johan Denoyer F4WAT (KISS TNC app, GitHub `jdenoy`).

- [Discussion #764](https://github.com/armel/uv-k1-k5v3-firmware-custom/discussions/764)
  (Ideas). Posted on 2026-11-02 as issue #763 from [f4hwn-kisstnc-issue.md](f4hwn-kisstnc-issue.md):
  ACKMODE, a counter for dropped frames, USB flow control, identification and RSSI. Our
  follow-up the same day introduced AXLink and AXLink TNC (commits they can take with
  `git am`) and mentioned the truncated frame after a USB re-enumeration. No answer yet
  (checked 2026-11-03). The next reply, with the commits, is drafted in
  [f4hwn-764-followup-2.md](f4hwn-764-followup-2.md).

### What we told them

| Statement | Status |
|---|---|
| The app holds one frame; host frames that arrive while it waits are dropped without a counter. | still true for KISS TNC 0.9; AXLink TNC counts them (ce3a675) |
| A KISS frame with 256 payload bytes laps the 256-byte USB ring if written at once. | still true (source) |
| AXLink paces its writes and works with the current app; tested in a simulator. | still true, and better since (below) |
| A frame cut off by a USB re-enumeration goes out truncated. | still true for KISS TNC 0.9; AXLink TNC drops it (b9537d4); AXLink sends a resync on reconnect |
| USB flow control belongs to the resident firmware. | still true |

### To tell them next time

- Since 2026-11-02 AXLink also waits longer after the radio heard another station, and its
  transmit queue merges or drops TCP repeats. In the model the original app then drops no frame
  in a 2 KB download (39 s; with ACKMODE 38 s). ACKMODE still covers what the host cannot see:
  a station still transmitting when the wait ends, and signals the radio could not decode.
  Confirm on the bench before telling them.
- Results of the bench test with two UV-K1.
- The commits they can take (in the draft): `b8c3573` (host harness: drawing log, held-back
  host bytes), `4a25af6` ACKMODE, `ce3a675` lost counter, `b9537d4` USB reset. Checked on
  2026-11-03: `git format-patch --relative=app` of these four, `git am
  --directory=App/apps/kisstnc` on F4HWN v6.3.0, and `test/test_tnc.py` there passes (28
  checks). The rename to AXLink TNC (`f310a5e`) is not part of it.
