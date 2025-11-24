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
	IPVersion      int      `yaml:"ipVersion"` // 0 = auto, 4, or 6
}

type Config struct {
	Subnets                     []SubnetConfig `yaml:"subnets"`
	Targets                     []string       `yaml:"targets"`     // legacy, IPv4 default
	TargetsIPv4                 []string       `yaml:"targetsIPv4"` // new
	TargetsIPv6                 []string       `yaml:"targetsIPv6"` // new
	IPsPerSubnet                int            `yaml:"ipsPerSubnet"`
	IntervalSeconds             int            `yaml:"intervalSeconds"`
	AutoMountSubnets            bool           `yaml:"autoMountSubnets"`
	DefaultInterface            string         `yaml:"defaultInterface"`
	RunFailureScripts           bool           `yaml:"runFailureScripts"`
	FailureScriptsDir           string         `yaml:"failureScriptsDir"`
	AlertOnPartialTargetFailure bool           `yaml:"alertOnPartialTargetFailure"`
	MetricsAddr                 string         `yaml:"metricsAddr"`
	LogFormat                   string         `yaml:"logFormat"`
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
	if len(c.Targets) == 0 && len(c.TargetsIPv4) == 0 && len(c.TargetsIPv6) == 0 {
		c.Targets = append([]string(nil), defaultTargets...)
	}
	if len(c.TargetsIPv4) == 0 && len(c.Targets) > 0 {
		c.TargetsIPv4 = append([]string(nil), c.Targets...)
	}
	if c.IPsPerSubnet == 0 {
		c.IPsPerSubnet = 5
	}
	if c.IntervalSeconds == 0 {
		c.IntervalSeconds = 60
	}
	if c.FailureScriptsDir == "" {
		c.FailureScriptsDir = "./sh"
	}
	if c.LogFormat == "" {
		c.LogFormat = "text"
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
		isIPv4 := ip.To4() != nil
		version := subnet.IPVersion
		if version == 0 {
			// Auto-detect: accept either
			if isIPv4 {
				version = 4
			} else {
				version = 6
			}
		} else {
			// Explicit version must match CIDR family
			if version == 4 && !isIPv4 {
				return fmt.Errorf("subnet %s: ipVersion=4 but CIDR is IPv6", subnet.CIDR)
			}
			if version == 6 && isIPv4 {
				return fmt.Errorf("subnet %s: ipVersion=6 but CIDR is IPv4", subnet.CIDR)
			}
			if version != 4 && version != 6 {
				return fmt.Errorf("subnet %s: ipVersion must be 0, 4, or 6, got %d", subnet.CIDR, version)
			}
		}
		// For IPv4, keep existing mask validation (bits == 32, maskSize < 31)
		if isIPv4 {
			maskSize, bits := ipNet.Mask.Size()
			if bits != 32 {
				return fmt.Errorf("subnet %s must be ipv4", subnet.CIDR)
			}
			if maskSize >= 31 {
				return fmt.Errorf("subnet %s too small for host allocation", subnet.CIDR)
			}
		}
		// For IPv6, validate prefix length (reject /128)
		if !isIPv4 {
			maskSize, bits := ipNet.Mask.Size()
			if bits != 128 {
				return fmt.Errorf("subnet %s must be ipv6", subnet.CIDR)
			}
			if maskSize >= 128 {
				return fmt.Errorf("subnet %s prefix too small (must be /0-/127)", subnet.CIDR)
			}
		}
		// Validate exclude hosts
		for _, host := range subnet.ExcludeHosts {
			hostIP := net.ParseIP(host)
			if hostIP == nil {
				return fmt.Errorf("subnet %s has invalid exclude host %s", subnet.CIDR, host)
			}
			if !ipNet.Contains(hostIP) {
				return fmt.Errorf("subnet %s exclude host %s outside subnet", subnet.CIDR, host)
			}
		}
	}
	// Require at least one target list
	if len(c.Targets) == 0 && len(c.TargetsIPv4) == 0 && len(c.TargetsIPv6) == 0 {
		return errors.New("no targets configured (must specify targets, targetsIPv4, or targetsIPv6)")
	}
	return nil
}
