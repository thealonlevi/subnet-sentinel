package subnets

import (
	"net"
	"testing"

	"github.com/thealonlevi/subnet-sentinel/internal/config"
)

func TestFromConfigsParsesSubnets(t *testing.T) {
	subnetConfigs := []config.SubnetConfig{
		{
			CIDR:           "192.168.10.0/24",
			ExcludeHosts:   []string{"192.168.10.1"},
			MountInterface: "eth0",
		},
	}
	result, err := FromConfigs(subnetConfigs)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 subnet, got %d", len(result))
	}
	if result[0].CIDR != subnetConfigs[0].CIDR {
		t.Fatalf("expected cidr %s, got %s", subnetConfigs[0].CIDR, result[0].CIDR)
	}
	if result[0].MountInterface != "eth0" {
		t.Fatalf("expected interface eth0, got %s", result[0].MountInterface)
	}
	if len(result[0].ExcludeHosts) != 1 {
		t.Fatalf("expected 1 exclude host, got %d", len(result[0].ExcludeHosts))
	}
	if !result[0].Network.Contains(net.ParseIP("192.168.10.5")) {
		t.Fatalf("expected parsed subnet to contain host")
	}
}

func TestRandomHostsRespectsExclusions(t *testing.T) {
	_, ipNet, err := net.ParseCIDR("10.0.0.0/29")
	if err != nil {
		t.Fatalf("parse cidr: %v", err)
	}
	excludes := []net.IP{
		net.ParseIP("10.0.0.1"),
	}
	hosts, err := RandomHosts(ipNet, excludes, 2)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(hosts))
	}
	seen := make(map[string]struct{})
	for _, host := range hosts {
		if host == nil {
			t.Fatalf("host nil")
		}
		if host.Equal(net.ParseIP("10.0.0.0")) {
			t.Fatalf("network address selected")
		}
		if host.Equal(net.ParseIP("10.0.0.7")) {
			t.Fatalf("broadcast address selected")
		}
		if host.Equal(net.ParseIP("10.0.0.1")) {
			t.Fatalf("excluded host selected")
		}
		key := host.String()
		if _, ok := seen[key]; ok {
			t.Fatalf("duplicate host selected")
		}
		seen[key] = struct{}{}
		if !ipNet.Contains(host) {
			t.Fatalf("host outside subnet")
		}
	}
}

func TestFromConfigsParsesIPv6Subnets(t *testing.T) {
	subnetConfigs := []config.SubnetConfig{
		{
			CIDR:           "2a0f:b243::/32",
			ExcludeHosts:   []string{"2a0f:b243::1"},
			MountInterface: "lo",
			IPVersion:      6,
		},
	}
	result, err := FromConfigs(subnetConfigs)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 subnet, got %d", len(result))
	}
	if result[0].IPVersion != 6 {
		t.Fatalf("expected IPVersion=6, got %d", result[0].IPVersion)
	}
	if result[0].Network.IP.To4() != nil {
		t.Fatalf("expected IPv6 address, got IPv4")
	}
	if !result[0].Network.Contains(net.ParseIP("2a0f:b243::5")) {
		t.Fatalf("expected parsed subnet to contain host")
	}
	if len(result[0].ExcludeHosts) != 1 {
		t.Fatalf("expected 1 exclude host, got %d", len(result[0].ExcludeHosts))
	}
}

func TestFromConfigsAutoDetectsIPv6(t *testing.T) {
	subnetConfigs := []config.SubnetConfig{
		{
			CIDR:           "2a0f:b243::/32",
			MountInterface: "lo",
			IPVersion:      0, // auto-detect
		},
	}
	result, err := FromConfigs(subnetConfigs)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result[0].IPVersion != 6 {
		t.Fatalf("expected auto-detected IPVersion=6, got %d", result[0].IPVersion)
	}
}

func TestRandomHostsIPv6(t *testing.T) {
	_, ipNet, err := net.ParseCIDR("2a0f:b243::/64")
	if err != nil {
		t.Fatalf("parse cidr: %v", err)
	}
	excludes := []net.IP{
		net.ParseIP("2a0f:b243::1"),
	}
	hosts, err := RandomHosts(ipNet, excludes, 3)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(hosts) != 3 {
		t.Fatalf("expected 3 hosts, got %d", len(hosts))
	}
	seen := make(map[string]struct{})
	for _, host := range hosts {
		if host == nil {
			t.Fatalf("host nil")
		}
		if host.To4() != nil {
			t.Fatalf("expected IPv6 address, got IPv4")
		}
		if host.Equal(net.ParseIP("2a0f:b243::")) {
			t.Fatalf("network address selected")
		}
		if host.Equal(net.ParseIP("2a0f:b243::1")) {
			t.Fatalf("excluded host selected")
		}
		key := host.String()
		if _, ok := seen[key]; ok {
			t.Fatalf("duplicate host selected")
		}
		seen[key] = struct{}{}
		if !ipNet.Contains(host) {
			t.Fatalf("host outside subnet")
		}
	}
}

func TestDeterministicHostIPv6(t *testing.T) {
	_, ipNet, err := net.ParseCIDR("2a0f:b243::/64")
	if err != nil {
		t.Fatalf("parse cidr: %v", err)
	}
	excludes := []net.IP{
		net.ParseIP("2a0f:b243::1"),
	}
	host, err := DeterministicHost(ipNet, excludes)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if host == nil {
		t.Fatalf("expected host, got nil")
	}
	if host.To4() != nil {
		t.Fatalf("expected IPv6 address, got IPv4")
	}
	if host.Equal(net.ParseIP("2a0f:b243::")) {
		t.Fatalf("network address selected")
	}
	if host.Equal(net.ParseIP("2a0f:b243::1")) {
		t.Fatalf("excluded host selected")
	}
	if !ipNet.Contains(host) {
		t.Fatalf("host outside subnet")
	}
}
