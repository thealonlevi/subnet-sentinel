package config

import (
	"errors"
	"fmt"
	"net"
	"os"

	"gopkg.in/yaml.v3"
)

type SubnetConfig struct {
	CIDR           string   `yaml:"cidr"`
	ExcludeHosts   []string `yaml:"excludeHosts"`
	MountInterface string   `yaml:"mountInterface"`
}

type Config struct {
	Subnets          []SubnetConfig `yaml:"subnets"`
	Targets          []string       `yaml:"targets"`
	IPsPerSubnet     int            `yaml:"ipsPerSubnet"`
	IntervalSeconds  int            `yaml:"intervalSeconds"`
	AutoMountSubnets bool           `yaml:"autoMountSubnets"`
	DefaultInterface string         `yaml:"defaultInterface"`
}

var defaultTargets = []string{
	"https://google.com",
	"https://ipinfo.io",
	"https://icanhazip.com",
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) applyDefaults() {
	if len(c.Targets) == 0 {
		c.Targets = append([]string(nil), defaultTargets...)
	}
	if c.IPsPerSubnet == 0 {
		c.IPsPerSubnet = 5
	}
	if c.IntervalSeconds == 0 {
		c.IntervalSeconds = 60
	}
}

func (c Config) Validate() error {
	if len(c.Subnets) == 0 {
		return errors.New("no subnets configured")
	}
	if c.IPsPerSubnet < 0 {
		return errors.New("ipsPerSubnet must be positive")
	}
	if c.IPsPerSubnet == 0 {
		return errors.New("ipsPerSubnet must be positive")
	}
	if c.IntervalSeconds < 0 {
		return errors.New("intervalSeconds must be non-negative")
	}
	for i, subnet := range c.Subnets {
		if subnet.CIDR == "" {
			return fmt.Errorf("subnet %d missing cidr", i)
		}
		ip, ipNet, err := net.ParseCIDR(subnet.CIDR)
		if err != nil {
			return fmt.Errorf("subnet %s invalid cidr: %w", subnet.CIDR, err)
		}
		if ip.To4() == nil || len(ipNet.Mask) != net.IPv4len {
			return fmt.Errorf("subnet %s must be ipv4", subnet.CIDR)
		}
		for _, host := range subnet.ExcludeHosts {
			hostIP := net.ParseIP(host)
			if hostIP == nil || hostIP.To4() == nil {
				return fmt.Errorf("subnet %s has invalid exclude host %s", subnet.CIDR, host)
			}
		}
	}
	if len(c.Targets) == 0 {
		return errors.New("no targets configured")
	}
	return nil
}
