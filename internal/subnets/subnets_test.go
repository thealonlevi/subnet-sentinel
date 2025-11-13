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
