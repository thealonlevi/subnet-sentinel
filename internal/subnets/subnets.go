package subnets

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math"
	mathrand "math/rand"
	"net"

	"github.com/thealonlevi/subnet-sentinel/internal/config"
)

type Subnet struct {
	CIDR           string
	Network        *net.IPNet
	ExcludeHosts   []net.IP
	MountInterface string
}

func FromConfigs(configs []config.SubnetConfig) ([]Subnet, error) {
	result := make([]Subnet, 0, len(configs))
	for _, cfg := range configs {
		ip, ipNet, err := net.ParseCIDR(cfg.CIDR)
		if err != nil {
			return nil, fmt.Errorf("parse subnet %s: %w", cfg.CIDR, err)
		}
		ip = ip.To4()
		if ip == nil {
			return nil, fmt.Errorf("subnet %s must be ipv4", cfg.CIDR)
		}
		maskSize, bits := ipNet.Mask.Size()
		if bits != 32 {
			return nil, fmt.Errorf("subnet %s must be ipv4", cfg.CIDR)
		}
		if maskSize >= 31 {
			return nil, fmt.Errorf("subnet %s too small for host allocation", cfg.CIDR)
		}
		ipNet.IP = ip
		excludes := make([]net.IP, 0, len(cfg.ExcludeHosts))
		for _, host := range cfg.ExcludeHosts {
			hostIP := net.ParseIP(host)
			if hostIP == nil || hostIP.To4() == nil {
				return nil, fmt.Errorf("subnet %s invalid exclude host %s", cfg.CIDR, host)
			}
			hostIP = hostIP.To4()
			if !ipNet.Contains(hostIP) {
				return nil, fmt.Errorf("subnet %s exclude host %s outside subnet", cfg.CIDR, host)
			}
			excludes = append(excludes, append(net.IP(nil), hostIP...))
		}
		result = append(result, Subnet{
			CIDR:           cfg.CIDR,
			Network:        ipNet,
			ExcludeHosts:   excludes,
			MountInterface: cfg.MountInterface,
		})
	}
	return result, nil
}

func RandomHosts(ipNet *net.IPNet, excludes []net.IP, count int) ([]net.IP, error) {
	if count <= 0 {
		return nil, fmt.Errorf("count must be positive")
	}
	network := ipNet.IP.Mask(ipNet.Mask).To4()
	if network == nil {
		return nil, fmt.Errorf("only ipv4 supported")
	}
	maskSize, bits := ipNet.Mask.Size()
	if bits != 32 {
		return nil, fmt.Errorf("only ipv4 supported")
	}
	hostBits := uint32(32 - maskSize)
	hostCount := uint32(1 << hostBits)
	if hostCount <= 2 {
		return nil, fmt.Errorf("subnet %s has no assignable hosts", ipNet.String())
	}
	networkVal := ipToUint32(network)
	firstHost := networkVal + 1
	lastHost := networkVal + hostCount - 2
	excludeSet := make(map[uint32]struct{})
	for _, ip := range excludes {
		ip4 := ip.To4()
		if ip4 == nil {
			continue
		}
		val := ipToUint32(ip4)
		if val <= networkVal || val >= networkVal+hostCount-1 {
			continue
		}
		excludeSet[val] = struct{}{}
	}
	available := int(hostCount-2) - len(excludeSet)
	if available < count {
		return nil, fmt.Errorf("subnet %s does not have enough available hosts", ipNet.String())
	}
	seedBytes := make([]byte, 8)
	if _, err := rand.Read(seedBytes); err != nil {
		return nil, fmt.Errorf("seed randomness: %w", err)
	}
	seed := int64(binary.LittleEndian.Uint64(seedBytes))
	r := mathrand.New(mathrand.NewSource(seed))
	results := make([]net.IP, 0, count)
	used := make(map[uint32]struct{})
	maxAttempts := int(math.Max(float64(count*20), 100))
	attempts := 0
	for len(results) < count {
		if attempts > maxAttempts {
			return nil, fmt.Errorf("failed to select enough hosts for %s", ipNet.String())
		}
		attempts++
		candidateVal := firstHost + uint32(r.Int63n(int64(lastHost-firstHost+1)))
		if candidateVal <= networkVal || candidateVal >= networkVal+hostCount-1 {
			continue
		}
		if _, ok := excludeSet[candidateVal]; ok {
			continue
		}
		if _, ok := used[candidateVal]; ok {
			continue
		}
		used[candidateVal] = struct{}{}
		results = append(results, uint32ToIP(candidateVal))
	}
	return results, nil
}

func DeterministicHost(ipNet *net.IPNet, excludes []net.IP) (net.IP, error) {
	network := ipNet.IP.Mask(ipNet.Mask).To4()
	if network == nil {
		return nil, fmt.Errorf("only ipv4 supported")
	}
	maskSize, bits := ipNet.Mask.Size()
	if bits != 32 {
		return nil, fmt.Errorf("only ipv4 supported")
	}
	hostBits := uint32(32 - maskSize)
	hostCount := uint32(1 << hostBits)
	if hostCount <= 2 {
		return nil, fmt.Errorf("subnet %s has no assignable hosts", ipNet.String())
	}
	networkVal := ipToUint32(network)
	firstHost := networkVal + 1
	lastHost := networkVal + hostCount - 2
	excludeSet := make(map[uint32]struct{})
	for _, ip := range excludes {
		if ip4 := ip.To4(); ip4 != nil {
			val := ipToUint32(ip4)
			if val >= firstHost && val <= lastHost {
				excludeSet[val] = struct{}{}
			}
		}
	}
	start := firstHost + 4
	if start > lastHost {
		start = firstHost
	}
	for i := uint32(0); i <= lastHost-firstHost; i++ {
		candidate := start + i
		if candidate > lastHost {
			candidate = firstHost + (candidate - lastHost - 1)
		}
		if _, ok := excludeSet[candidate]; ok {
			continue
		}
		return uint32ToIP(candidate), nil
	}
	return nil, fmt.Errorf("no available host in %s", ipNet.String())
}

func ipToUint32(ip net.IP) uint32 {
	ip4 := ip.To4()
	return binary.BigEndian.Uint32(ip4)
}

func uint32ToIP(v uint32) net.IP {
	ip := make(net.IP, net.IPv4len)
	binary.BigEndian.PutUint32(ip, v)
	return ip
}
