# KISS simulator of the v0.1 lab

Part of the v0.1 lab with pdn/pdn-net ([HISTORY](../docs/HISTORY.md)). The Go core has its own
lab: `scripts/lab-netns.sh`.

Requirements: Docker with Compose, a clean pinned Packet.NET checkout and its release build
(`python3 scripts/audit-upstream.py --run`).

```sh
docker compose -f simulator/compose.yaml up -d --wait --wait-timeout 90
python3 scripts/test-kiss-simulator.py
docker compose -f simulator/compose.yaml down
```

Run the clean-up also after a failed test. Ports 8100, 8101 and 18080 on loopback must be free.
Do not stop any other container project. No USB devices, sound cards or serial ports are used.

The runner starts only
`NetsimKissTcpInterop.UI_Frame_Sent_On_Node_A_Is_Received_On_Node_B`. It requires exactly one
passed test; a skip counts as a failure. Result, TRX and test log go to
`artifacts/m0/kiss-simulator`.

The test uses the existing Packet.NET KISS and AX.25 parsers and the audio-domain simulator
`packethacking/net-sim` v0.2.0. The image is pinned by digest (from the pinned Packet.NET
compose file). The image metadata name source commit `872d78a81b068dc6d96035c9cb85e23b21c8945b`
and MIT as the licence; a full audit of the image and its dependencies is still open.

Our configuration reduces the upstream topology to two AFSK1200 nodes. Channel parameters for FM
capture and collisions are set, but their failure scenarios are not covered by this single
smoke test.

`compose.ip.yaml`, `node.Dockerfile` and `node.py` are the two-node IP lab
(`python3 scripts/test-two-nodes.py`).
