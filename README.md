# subnet-sentinel

`subnet-sentinel` is a daemon and CLI that probes outbound connectivity using random source IPs carved from configured IPv4 subnets. It helps operators verify that routed subnets remain usable and mounted correctly on the host.

## Features
- Periodic or one-shot connectivity checks against configurable HTTP targets
- Source-IP binding per request with per-target latency and status reporting
- Optional auto-mounting of subnets: interface IP assignment, loopback routes, and `ip_nonlocal_bind`
- CLI for running checks, ensuring mounts, and inspecting mount status
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
autoMountSubnets: true
defaultInterface: eno1
```

Key fields:
- `subnets`: CIDRs to monitor, with optional host exclusions and interface overrides
- `targets`: HTTP endpoints to probe (defaults to public connectivity targets)
- `ipsPerSubnet`: number of unique hosts sampled per subnet per run (default 5)
- `intervalSeconds`: delay between runs in daemon mode (default 60)
- `autoMountSubnets`: enable auto-mounting before checks (`false` by default)
- `defaultInterface`: interface used for mounting when a subnet override is absent

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

## Testing
```bash
go test ./...
```

