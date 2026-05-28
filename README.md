# Sunrise Tech Check

A cross-platform desktop tool that checks whether your machine and network are ready for remote work via Sunrise. Run before a session to surface latency, throughput, jitter, MTU, DNS, and clock-skew problems before they bite live.

## What it does

1. Asks for your name and email on a short consent screen (pre-filled from the last run on this machine).
2. Collects machine information (OS, CPU, RAM, GPU, displays, USB count, network adapters with Wi-Fi detection, VPN/AV/firewall state, a stable machine ID).
3. Fetches a city-level client location via ipinfo.io.
4. Runs network measurements against every Sunrise region sequentially: latency, down/up throughput, jitter under load, MTU, clock skew, UDP 4172 reachability, and DNS timing.
5. Uploads the structured report to Sunrise's ingest service. A copy saves to your Desktop either way.

Takes about three minutes. You can cancel at any time.

## What it does NOT do

- No reading of personal files, documents, or browsing history.
- No screen recording or screen-content access.
- No keystroke logging.
- No microphone or camera access.
- No credential access (passwords, tokens, keychains).
- No always-on telemetry — the tool only uploads when you explicitly start a run and consent.

The uploaded report is the structured payload described above and nothing else. The source in this repository is the source that produces every signed binary distributed to the Sunrise team. You can rebuild and run from source to verify byte-for-byte.

## Install

Production builds for the Sunrise team are distributed via an internal auto-updater. The signed binaries the team installs are built from this repository.

For verification or contributions, build from source. See below.

## Build from source

Prerequisites: Go 1.22+, the Wails v2 CLI (`go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0`), and `create-dmg` (`brew install create-dmg`) for the macOS DMG package.

```bash
cd cmd/techcheck && wails dev     # live-reload dev window
make build-desktop                # macOS DMGs (arm64, x64) + Windows ZIP
go test ./...                     # all unit tests
go test ./cmd/techcheck/tests/e2e/... -v -timeout 120s    # end-to-end against local mocks
```

A source build with no `private/secrets.env` will compile and run, but uploads to the production Sunrise ingest fail without authentication. Point the tool at your own ingest via the sidecar config or provide your own `private/secrets.env` (see `private/secrets.env.example`).

## Companion server

The regional probe servers the client tests against are in [`techcheck-beacons`](https://github.com/sunriseproductions/techcheck-beacons).

## License

MIT. See [LICENSE](LICENSE).
