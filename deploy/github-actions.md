# GitHub Actions deploy

This repository now supports a minimal production deploy workflow at `.github/workflows/deploy.yml`.

## Scope

The workflow keeps the current production layout unchanged:

- builds one `linux/amd64` binary from `./cmd/swim-bot`
- runs `go test ./...` before deploy
- uploads only the binary to the VPS
- backs up the current binary in `/opt/swim-bot/backups/`
- installs the new binary to `/opt/swim-bot/swim-bot`
- restarts `swim-bot.service`
- verifies service status and recent logs

The workflow does not modify:

- `/opt/swim-bot/.env`
- `/opt/swim-bot/swim-bot.db`
- systemd unit layout
- reverse proxy config

## Trigger

Use `workflow_dispatch` from GitHub Actions for production releases.

This is intentionally manual for the first iteration.

## Required GitHub secrets

Create these repository or environment secrets:

- `PROD_HOST` — `datspike.xyz`
- `PROD_PORT` — `51022`
- `PROD_USER` — `spike`
- `PROD_SSH_KEY` — private SSH key allowed to connect as the deploy user
- `PROD_KNOWN_HOSTS` — output of `ssh-keyscan -p 51022 datspike.xyz`

Recommended: store them in the GitHub Actions environment `production` and require manual approval there.

## Server requirements

The deploy user must be able to run this without an interactive password prompt:

```bash
sudo systemctl restart swim-bot.service
```

Current production state already satisfies this.

## First-run checklist

1. Add the `production` environment in GitHub.
2. Add the required secrets.
3. Optionally require environment approval before the job can run.
4. Run the workflow manually on `master`.
5. Confirm `swim-bot.service` is active and review the deployment logs.

## Rollback

The workflow keeps the previous binary in `/opt/swim-bot/backups/`.

Manual rollback on the server:

```bash
cp /opt/swim-bot/backups/swim-bot-<timestamp> /opt/swim-bot/swim-bot
sudo systemctl restart swim-bot.service
```
