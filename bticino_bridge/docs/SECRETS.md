# Secrets

All real credentials used by the bridge live in **one file**: `secrets.yaml`,
next to `config.yaml`. `config.yaml` itself is a template and contains no
credentials, so it can be shared, committed and published safely.

## How it works

At startup the bridge loads `config.yaml` and then overlays `secrets.yaml` on
top of it. `secrets.yaml` has the **same structure** as `config.yaml` and only
needs the secret keys:

```yaml
sip:
  domain: "<gateway-id>.bs.iotleg.com"
  password: "<SIP password or HA1>"
mqtt:
  username: "<broker user>"
  password: "<broker password>"
homekit:
  pin: "<8 digits>"
security:
  web_username: "<dashboard user>"
  web_password: "<dashboard password>"
notifications:
  telegram:
    bot_token: "<token>"
```

Create it from the example and restrict its permissions:

```bash
cp configs/secrets.example.yaml configs/secrets.yaml
chmod 600 configs/secrets.yaml
```

- The default location is `secrets.yaml` in the same directory as the file
  given to `-config`. Use `-secrets <path>` to point somewhere else.
- Key names must match `config.yaml` exactly; unknown keys are ignored.
- Keep only the keys you fill in. A key set to `""` blanks the value coming
  from `config.yaml`.
- `secrets.yaml` and any variant of it (`secrets.yml`, `secrets.yaml.bak`, …)
  are listed in `.gitignore`. Never commit them.
- The bridge logs a warning if the file is readable by group or others.
- An empty file is fine (it provides no keys). A file that cannot be read or
  parsed stops the bridge with `invalid secrets file …`.
- `secrets.yaml` is meant for credentials only. It can technically set any
  key of `config.yaml`; keep other settings out of it.

## Fail-closed startup

The bridge refuses to start when a secret required by an **enabled** subsystem
is missing, empty, or still a placeholder from the example file:

| Subsystem enabled | Required keys |
|---|---|
| `sip.enabled` (and `-enable-video`) | `sip.domain`, `sip.password` |
| `mqtt.enabled` | `mqtt.password` |
| `homekit.enabled` (and `-enable-homekit`) | `homekit.pin` |
| `web.enabled` with `security.web_auth_required: true` | `security.web_password` |
| `notifications.telegram.enabled` | `notifications.telegram.bot_token` |

The error lists every missing key at once:

```
missing required secrets: sip.domain, sip.password, homekit.pin — set them in secrets.yaml (see configs/secrets.example.yaml)
```

Disable a subsystem (in `config.yaml` or with its `-enable-*` flag) if you do
not want to provide its credentials. There is no bypass, including `-test`.

## Web dashboard

The dashboard never shows or edits secrets. Credential fields are read-only in
the Settings page, and saving or restoring the configuration from the
dashboard writes `config.yaml` without any secret values.

If `config.yaml` still contains credentials that `secrets.yaml` does not
provide (a device that has not been migrated yet), the dashboard refuses to
save or restore, because doing so would delete them. Migrate first (below).

Configuration backups (`backups/` next to `config.yaml`) are created with
`0600` permissions and are gitignored. Backups made before the migration
contain credentials; delete them once the migration is verified.

## Helper scripts

Scripts under `scripts/` take credentials from the environment and stop with a
clear message when a variable is missing, for example `MQTT_PASS` and
`SIP_DOMAIN`.

`make check-secrets` scans the tracked files for secret-shaped values (bare
32-hex digests, `<id>.bs.iotleg.com` domains, cloud SIP user names) and fails
if it finds any. Run it before committing.

## One-time migration of an existing device

Builds from this version on expect the credentials in `secrets.yaml`. Do this
**once**, ideally before deploying such a build.

What happens if you do not: a device whose `config.yaml` still holds the
credentials keeps working after a binary-only deploy, but the bridge logs a
warning at every start and the dashboard refuses to save the configuration.
A device that receives the template `config.yaml` without having a
`secrets.yaml` stops at startup naming the missing keys.

1. On the device, go to the bridge directory (`/home/bticino/cfg/extra/`) and
   back up the current file: `cp config.yaml config.yaml.pre-secrets`.
2. Create `secrets.yaml` there with the values currently in `config.yaml`:
   `sip.domain`, `sip.password`, `mqtt.username`, `mqtt.password`,
   `homekit.pin`, `security.web_username`, `security.web_password` (and the
   Telegram token if you use it).
3. `chmod 600 secrets.yaml`.
4. Deploy the new build and restart the bridge. Check the log: it must not
   contain `missing required secrets`.
5. Blank those fields in the device's `config.yaml` (set them to `""`), so the
   credentials exist in one place only. Then delete `config.yaml.pre-secrets`
   and the old files under `backups/`, which still contain them.

Which deploy path touches the configuration:

- `deploy-standard.sh` uploads only the binary and `VERSION`. It never touches
  `config.yaml` or `secrets.yaml`.
- `scripts/deploy_to_bticino.sh` also uploads `config.yaml`. Since the
  repository's `config.yaml` is now a template, only use it once
  `secrets.yaml` exists on the device.
- No deploy script uploads or removes `secrets.yaml`.
