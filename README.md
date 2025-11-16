# subnet-sentinel

`subnet-sentinel` is a daemon and CLI that probes outbound connectivity using random source IPs carved from configured IPv4 subnets. It helps operators verify that routed subnets remain usable on the host.

## Features
- Periodic or one-shot connectivity checks against configurable HTTP targets
- Source-IP binding per request with per-target latency and status reporting
- Optional auto-mount retry: on the first HTTP failure (when enabled) the tool mounts subnets and retries once
- CLI for running checks and inspecting mount status
- Systemd service unit for unattended operation

## Requirements
- Go (latest stable)
- Linux host with `ip` tooling for mounting features (development tested against Ubuntu 22.04+)

## Build
```bash
go build ./...
```

## Configuration
Settings are sourced from `config.yaml` by default, overridable via `--config` or `SUBNET_SENTINEL_CONFIG`.

```yaml
subnets:
  - cidr: 154.208.64.0/21
    excludeHosts:
      - 154.208.64.0
      - 154.208.64.1
      - 154.208.64.2
      - 154.208.64.3
    mountInterface: eno1
  - cidr: 154.208.112.0/21
    mountInterface: eno1

targets:
  - https://google.com
  - https://ipinfo.io
  - https://icanhazip.com

ipsPerSubnet: 5
intervalSeconds: 60
autoMountSubnets: false
defaultInterface: eno1
```

Key fields:
- `subnets`: CIDRs to monitor, with optional host exclusions and interface overrides
- `targets`: HTTP endpoints to probe (defaults to public connectivity targets)
- `ipsPerSubnet`: number of unique hosts sampled per subnet per run (default 5)
- `intervalSeconds`: delay between runs in daemon mode (default 60). If set to `0`, `subnet-sentinel` runs continuously with a minimum 1-second delay between runs to avoid busy-looping
- `autoMountSubnets`: when true, the first HTTP failure in a run triggers subnet mounting and a single retry of that request
- `defaultInterface`: interface used when a subnet does not specify `mountInterface`

## CLI Usage
```bash
subnet-sentinel run           # default daemon mode
subnet-sentinel once          # single run
subnet-sentinel check-mount   # inspect current mount status
subnet-sentinel mount         # enforce mount prerequisites
```

### Flags
- `--config`, `-c`: alternate config path
- `--log-level`: `debug`, `info`, or `error` (default `info`)

## Systemd Service
Install the binary under `/usr/local/bin/subnet-sentinel` and place `packaging/systemd/subnet-sentinel.service` in `/etc/systemd/system/`. Then run:
```bash
sudo systemctl daemon-reload
sudo systemctl enable subnet-sentinel
sudo systemctl start subnet-sentinel
```

## Operational Notes
- With `autoMountSubnets` disabled, configure addresses, local routes (`ip route add local ... dev lo`), and `ip_nonlocal_bind=1` manually before running the daemon.
- With `autoMountSubnets` enabled, the first HTTP failure in a run will assign deterministic host IPs, ensure loopback routes, set `ip_nonlocal_bind=1`, and retry the failed request once.

### Container vs bare-metal

- On bare metal (systemd unit):
  - The sample unit runs as `User=root` because mounting features require:
    - `ip addr` / `ip route` commands
    - write access to `/proc/sys/net/ipv4/ip_nonlocal_bind`
  - This is the recommended setup when using `autoMountSubnets: true`.

- In Docker:
  - The provided `Dockerfile` creates a non-root user (`appuser`) and runs the binary as that user by default.
  - In this default container configuration, subnet mounting (`autoMountSubnets`, `mount` and `check-mount` that modify system state) will fail unless the container is started with:
    - root or a user with sufficient privileges, and
    - appropriate capabilities (e.g. `--cap-add=NET_ADMIN` and write access to `/proc/sys/net/ipv4/ip_nonlocal_bind`).
  - For a "check-only" setup inside Docker (no mounting), disable `autoMountSubnets` and preconfigure IPs/routes on the host or via your orchestrator.

## Testing
```bash
go test ./...
```

