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
	IPVersion      int
}

func FromConfigs(configs []config.SubnetConfig) ([]Subnet, error) {
	result := make([]Subnet, 0, len(configs))
	for _, cfg := range configs {
		ip, ipNet, err := net.ParseCIDR(cfg.CIDR)
		if err != nil {
			return nil, fmt.Errorf("parse subnet %s: %w", cfg.CIDR, err)
		}
		isIPv4 := ip.To4() != nil
		version := cfg.IPVersion
		if version == 0 {
			// Auto-detect
			if isIPv4 {
				version = 4
			} else {
				version = 6
			}
		}
		// For IPv4, keep existing behavior
		if isIPv4 {
			maskSize, bits := ipNet.Mask.Size()
			if bits != 32 {
				return nil, fmt.Errorf("subnet %s must be ipv4", cfg.CIDR)
			}
			if maskSize >= 31 {
				return nil, fmt.Errorf("subnet %s too small for host allocation", cfg.CIDR)
			}
			ipNet.IP = ip.To4()
		} else {
			// For IPv6, validate prefix length
			maskSize, bits := ipNet.Mask.Size()
			if bits != 128 {
				return nil, fmt.Errorf("subnet %s must be ipv6", cfg.CIDR)
			}
			if maskSize >= 128 {
				return nil, fmt.Errorf("subnet %s prefix too small (must be /0-/127)", cfg.CIDR)
			}
			// Keep full 16-byte IPv6 address
		}
		excludes := make([]net.IP, 0, len(cfg.ExcludeHosts))
		for _, host := range cfg.ExcludeHosts {
			hostIP := net.ParseIP(host)
			if hostIP == nil {
				return nil, fmt.Errorf("subnet %s invalid exclude host %s", cfg.CIDR, host)
			}
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
			IPVersion:      version,
		})
	}
	return result, nil
}

func RandomHosts(ipNet *net.IPNet, excludes []net.IP, count int) ([]net.IP, error) {
	if count <= 0 {
		return nil, fmt.Errorf("count must be positive")
	}
	if ipNet.IP.To4() != nil {
		return randomHostsIPv4(ipNet, excludes, count)
	}
	return randomHostsIPv6(ipNet, excludes, count)
}

func randomHostsIPv4(ipNet *net.IPNet, excludes []net.IP, count int) ([]net.IP, error) {
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

func randomHostsIPv6(ipNet *net.IPNet, excludes []net.IP, count int) ([]net.IP, error) {
	maskSize, bits := ipNet.Mask.Size()
	if bits != 128 {
		return nil, fmt.Errorf("only ipv6 supported")
	}
	hostBits := 128 - maskSize
	if hostBits == 0 {
		if count > 1 {
			return nil, fmt.Errorf("subnet %s is /128, cannot select multiple hosts", ipNet.String())
		}
		// For /128, return the network address itself if not excluded
		network := ipNet.IP.Mask(ipNet.Mask)
		excludeSet := make(map[string]struct{})
		for _, ip := range excludes {
			excludeSet[ip.String()] = struct{}{}
		}
		if _, excluded := excludeSet[network.String()]; excluded {
			return nil, fmt.Errorf("subnet %s has no available hosts (all excluded)", ipNet.String())
		}
		return []net.IP{append(net.IP(nil), network...)}, nil
	}
	network := ipNet.IP.Mask(ipNet.Mask)
	excludeSet := make(map[string]struct{})
	for _, ip := range excludes {
		excludeSet[ip.String()] = struct{}{}
	}
	seedBytes := make([]byte, 8)
	if _, err := rand.Read(seedBytes); err != nil {
		return nil, fmt.Errorf("seed randomness: %w", err)
	}
	seed := int64(binary.LittleEndian.Uint64(seedBytes))
	r := mathrand.New(mathrand.NewSource(seed))
	results := make([]net.IP, 0, count)
	used := make(map[string]struct{})
	maxAttempts := int(math.Max(float64(count*20), 100))
	attempts := 0
	for len(results) < count {
		if attempts > maxAttempts {
			return nil, fmt.Errorf("failed to select enough hosts for %s", ipNet.String())
		}
		attempts++
		// Generate random host bits
		candidate := make(net.IP, 16)
		copy(candidate, network)
		// Randomize bytes in the host portion
		startByte := maskSize / 8
		bitsInStartByte := maskSize % 8
		// Handle the first byte if it's partially in host space
		if bitsInStartByte > 0 && startByte < 16 {
			mask := byte(0xFF >> bitsInStartByte)
			candidate[startByte] = network[startByte] | (byte(r.Intn(256)) & mask)
			startByte++
		}
		// Randomize fully host bytes
		for i := startByte; i < 16; i++ {
			candidate[i] = byte(r.Intn(256))
		}
		// Ensure we don't use network address
		if candidate.Equal(network) {
			continue
		}
		candidateStr := candidate.String()
		if _, excluded := excludeSet[candidateStr]; excluded {
			continue
		}
		if _, alreadyUsed := used[candidateStr]; alreadyUsed {
			continue
		}
		if !ipNet.Contains(candidate) {
			continue
		}
		used[candidateStr] = struct{}{}
		results = append(results, candidate)
	}
	return results, nil
}

func DeterministicHost(ipNet *net.IPNet, excludes []net.IP) (net.IP, error) {
	if ipNet.IP.To4() != nil {
		return deterministicHostIPv4(ipNet, excludes)
	}
	return deterministicHostIPv6(ipNet, excludes)
}

func deterministicHostIPv4(ipNet *net.IPNet, excludes []net.IP) (net.IP, error) {
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
	start := networkVal + 4
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

func deterministicHostIPv6(ipNet *net.IPNet, excludes []net.IP) (net.IP, error) {
	_, bits := ipNet.Mask.Size()
	if bits != 128 {
		return nil, fmt.Errorf("only ipv6 supported")
	}
	network := ipNet.IP.Mask(ipNet.Mask)
	excludeSet := make(map[string]struct{})
	for _, ip := range excludes {
		excludeSet[ip.String()] = struct{}{}
	}
	// Start with network + 1 in host bits
	candidate := make(net.IP, 16)
	copy(candidate, network)
	// Add 1 to the last byte (deterministic offset)
	if len(candidate) > 0 {
		candidate[15]++
		// Handle overflow
		for i := 15; i >= 0 && candidate[i] == 0; i-- {
			if i > 0 {
				candidate[i-1]++
			}
		}
	}
	// Try candidate and wrap around if needed
	startCandidate := append(net.IP(nil), candidate...)
	maxIterations := 1000 // Safety limit
	for i := 0; i < maxIterations; i++ {
		// Skip network address
		if candidate.Equal(network) {
			// Increment
			for j := 15; j >= 0; j-- {
				candidate[j]++
				if candidate[j] != 0 {
					break
				}
			}
			continue
		}
		if !ipNet.Contains(candidate) {
			// Reset to start of host space
			copy(candidate, network)
			candidate[15]++
			continue
		}
		candidateStr := candidate.String()
		if _, excluded := excludeSet[candidateStr]; !excluded {
			return candidate, nil
		}
		// Increment for next iteration
		for j := 15; j >= 0; j-- {
			candidate[j]++
			if candidate[j] != 0 {
				break
			}
		}
		// Check if we've wrapped around
		if candidate.Equal(startCandidate) {
			break
		}
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
