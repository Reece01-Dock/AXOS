# SSH access to the GT-AX6000

As of Milestone 2 (2026-09-23):

- SSH is **key-only** (`sshd_pass=0`). Password login is rejected.
- Dropbear listens on LAN port 22.
- Install root for AXOS software: `/jffs/axos` (no USB required).
- `axosd` Core API: `127.0.0.1:9090` (loopback only — tunnel via SSH).

## Connecting from a workstation

```sh
ssh -i ~/.ssh/axos_gtax6000_ed25519 Reece@192.168.50.1
# or add to ~/.ssh/config:
# Host axos-router
#   HostName 192.168.50.1
#   User Reece
#   IdentityFile ~/.ssh/axos_gtax6000_ed25519
#   IdentitiesOnly yes
```

The private key is **not** in this repo. Generate/install with:

```sh
ssh-keygen -t ed25519 -f ~/.ssh/axos_gtax6000_ed25519 -N ''
# then set nvram sshd_authkeys to the .pub contents and restart_sshd
```

## Tunnel the Core API

```sh
ssh -i ~/.ssh/axos_gtax6000_ed25519 -L 9090:127.0.0.1:9090 Reece@192.168.50.1
# then: curl http://127.0.0.1:9090/healthz
```
