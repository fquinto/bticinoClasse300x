# Deploying to the device

Authoritative guide for deploying the bridge to a real BTicino Classe 300X.
Supersedes the older `DEPLOYMENT_GUIDE.md` / `DEPLOY_REAL_INSTRUCTIONS.md`
(kept for history; they describe an earlier, broken flow).

## One command

`deploy-standard.sh` lives at the **project root** (one level above
`bticino_bridge/`). From there:

```bash
./deploy-standard.sh full
```

(In-tree alternative: `make deploy` from `bticino_bridge/`, which builds and runs
`scripts/deploy.sh`.)

That runs the whole robust flow:

1. **build** — cross-compile ARMv7 (`GOOS=linux GOARCH=arm GOARM=7`, stripped).
2. **upload + verify** — stream the binary over SSH as base64 to
   `bticino_bridge.new`, then compare local vs remote **md5** (aborts on mismatch,
   without touching the running binary).
3. **backup** — copy the current binary to `bticino_bridge.prev` (rollback slot).
4. **stop** — kill *all* running instances (TERM, then KILL), verify 0 remain.
5. **swap** — `mv` the new binary into place, update `VERSION`.
6. **start** — launch exactly **one** instance with `setsid` (survives the SSH
   session, no dangling `sh`, no duplicate instances).
7. **health-check** — poll `http://<device>:8082/api/status` for the new version.
   If it doesn't come up (or more than one instance is running), it **rolls back
   automatically** to `bticino_bridge.prev`.

## Subcommands

| Command | What it does |
|---|---|
| `./deploy-standard.sh build` | Cross-compile the ARM binary only |
| `./deploy-standard.sh deploy` | Upload+verify+swap+start+health (needs a prior build) |
| `./deploy-standard.sh full` | build + deploy |
| `./deploy-standard.sh web` | Build **and** deploy the Svelte frontend only (no binary, no restart) |
| `./deploy-standard.sh status` | Version + instance count on the device |
| `./deploy-standard.sh logs` | Tail the runtime log (`/tmp/bridge_deploy.log`) |
| `./deploy-standard.sh restart` | Restart (single instance) |
| `./deploy-standard.sh rollback` | Restore `bticino_bridge.prev` |
| `./deploy-standard.sh stop` | Stop the bridge |

Add `--web` to `build`, `deploy` or `full` to also build/deploy the frontend in
the same run, e.g. `./deploy-standard.sh full --web`.

### Frontend (Svelte) deployment

The SPA is served **from disk** at `/home/bticino/cfg/extra/web/` (root:
`index.html` + `assets/`), so a binary-only deploy does **not** update it. Use
`web` / `--web`: it runs `vite build` (→ `web/dist`), clears the old hashed
assets on the device, and base64-copies the new `index.html` + `assets/*` over.
No restart is needed — static files are picked up immediately (hard-refresh the
browser, Ctrl/Cmd+Shift+R, to bypass cache).

## Device facts you must respect

- **Binary name is `bticino_bridge`** (underscore) at `/home/bticino/cfg/extra/`,
  launched as `./bticino_bridge` with the default `configs/config.yaml`.
- Reached via the SSH alias **`bticino`** (`~/.ssh/config`, `192.168.1.38`).
- **No `scp`, `curl`, `git`, `go`** on the device — file transfer is base64 over
  SSH; the device has `wget`, `base64`, `python3`, `netcat`, busybox `ps`.
- A **boot autostart** relaunches the bridge after a reboot. Don't assume nothing
  respawns it at boot; the deploy script starts it manually for in-place upgrades.
- The system has a **hardware watchdog** — a command storm against the native
  processes can make it reboot. Keep video off by default (see `README.md`).

## Manual fallback

If you ever need to do it by hand (the pattern the script automates):

```bash
# 1. Upload + verify (SEPARATE step — never combine upload and swap)
base64 bticino_bridge | ssh bticino \
  'cd /home/bticino/cfg/extra && base64 -d > bticino_bridge.new && chmod +x bticino_bridge.new && md5sum bticino_bridge.new'
md5sum bticino_bridge          # compare the two

# 2. Backup, stop all, swap, start ONE instance
ssh bticino 'cd /home/bticino/cfg/extra &&
  cp -a bticino_bridge bticino_bridge.prev &&
  for p in $(ps | grep bticino_bridge | grep -v grep | awk "{print \$1}"); do kill -9 $p; done &&
  mv bticino_bridge.new bticino_bridge &&
  setsid sh -c "exec ./bticino_bridge > /tmp/bridge_deploy.log 2>&1" < /dev/null > /dev/null 2>&1 &'

# 3. Verify
ssh bticino 'wget -qO- http://127.0.0.1:8082/api/status'
```

## Rollback

Automatic on a failed health-check. Manual:

```bash
./deploy-standard.sh rollback     # restores bticino_bridge.prev
```

The device also keeps `bticino_bridge.v0.15.5.bak` (a known-good baseline) if you
need to go further back.
