package config

import (
	"testing"
)

func TestConfigIPv6Validation(t *testing.T) {
	// IPv6 subnet with explicit ipVersion: 6
	cfg := Config{
		Subnets: []SubnetConfig{
			{CIDR: "2a0f:b243::/32", IPVersion: 6},
		},
		TargetsIPv6:  []string{"https://ifconfig.co"},
		IPsPerSubnet: 5,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid IPv6 config, got error: %v", err)
	}
}

func TestConfigIPv6AutoDetect(t *testing.T) {
	// IPv6 subnet with ipVersion: 0 (auto-detect)
	cfg := Config{
		Subnets: []SubnetConfig{
			{CIDR: "2a0f:b243::/32", IPVersion: 0},
		},
		TargetsIPv6:  []string{"https://ifconfig.co"},
		IPsPerSubnet: 5,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid IPv6 config with auto-detect, got error: %v", err)
	}
}

func TestConfigIPv6VersionMismatch(t *testing.T) {
	// IPv6 CIDR with ipVersion: 4 should fail
	cfg := Config{
		Subnets: []SubnetConfig{
			{CIDR: "2a0f:b243::/32", IPVersion: 4},
		},
		TargetsIPv6:  []string{"https://ifconfig.co"},
		IPsPerSubnet: 5,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for IPv6 CIDR with ipVersion=4")
	}
}

func TestConfigIPv4VersionMismatch(t *testing.T) {
	// IPv4 CIDR with ipVersion: 6 should fail
	cfg := Config{
		Subnets: []SubnetConfig{
			{CIDR: "192.168.1.0/24", IPVersion: 6},
		},
		TargetsIPv4:  []string{"https://google.com"},
		IPsPerSubnet: 5,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for IPv4 CIDR with ipVersion=6")
	}
}

func TestConfigBackwardCompatibility(t *testing.T) {
	// Legacy config with only targets (no TargetsIPv4/TargetsIPv6)
	cfg := Config{
		Subnets: []SubnetConfig{
			{CIDR: "192.168.1.0/24"},
		},
		Targets:      []string{"https://google.com"},
		IPsPerSubnet: 5,
	}
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid legacy config, got error: %v", err)
	}
	// TargetsIPv4 should be auto-populated
	if len(cfg.TargetsIPv4) == 0 {
		t.Fatalf("expected TargetsIPv4 to be auto-populated from Targets")
	}
	if len(cfg.TargetsIPv4) != len(cfg.Targets) {
		t.Fatalf("expected TargetsIPv4 to match Targets length")
	}
}

func TestConfigNoTargetsError(t *testing.T) {
	// Config with no targets at all should fail
	cfg := Config{
		Subnets: []SubnetConfig{
			{CIDR: "192.168.1.0/24"},
		},
		IPsPerSubnet: 5,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error when no targets configured")
	}
}

func TestConfigIPv6ExcludeHosts(t *testing.T) {
	// IPv6 subnet with IPv6 exclude hosts
	cfg := Config{
		Subnets: []SubnetConfig{
			{
				CIDR:         "2a0f:b243::/32",
				IPVersion:    6,
				ExcludeHosts: []string{"2a0f:b243::1", "2a0f:b243::2"},
			},
		},
		TargetsIPv6:  []string{"https://ifconfig.co"},
		IPsPerSubnet: 5,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid IPv6 config with exclude hosts, got error: %v", err)
	}
}
