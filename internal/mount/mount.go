package mount

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/thealonlevi/subnet-sentinel/internal/subnets"
)

type Request struct {
	Subnet    subnets.Subnet
	Interface string
}

type Status struct {
	CIDR         string
	Interface    string
	IPAssigned   bool
	RouteExists  bool
	NonLocalBind bool
	MountIP      net.IP
	Actions      []string
	Errors       []string
}

func PrepareRequests(defaultInterface string, subs []subnets.Subnet) []Request {
	result := make([]Request, 0, len(subs))
	for _, subnet := range subs {
		iface := subnet.MountInterface
		if iface == "" {
			iface = defaultInterface
		}
		result = append(result, Request{
			Subnet:    subnet,
			Interface: iface,
		})
	}
	return result
}

func Check(ctx context.Context, requests []Request) ([]Status, error) {
	statuses := make([]Status, 0, len(requests))
	nonLocalStatus, nonLocalErr := readNonLocalBind()
	var firstErr error
	for _, req := range requests {
		isIPv4 := req.Subnet.Network.IP.To4() != nil
		nonLocal := false
		if isIPv4 {
			nonLocal = nonLocalStatus.IPv4Enabled
		} else {
			nonLocal = nonLocalStatus.IPv6Enabled
		}
		status := Status{
			CIDR:         req.Subnet.CIDR,
			Interface:    req.Interface,
			NonLocalBind: nonLocal,
		}
		if nonLocalErr != nil && firstErr == nil {
			firstErr = nonLocalErr
		}
		if nonLocalErr != nil {
			status.Errors = append(status.Errors, fmt.Sprintf("nonlocal bind check failed: %v", nonLocalErr))
		}
		if req.Interface == "" {
			status.Errors = append(status.Errors, "no interface configured")
			statuses = append(statuses, status)
			continue
		}
		assigned, err := interfaceHasSubnetIP(ctx, req.Interface, req.Subnet.Network)
		if err != nil {
			status.Errors = append(status.Errors, fmt.Sprintf("ip check failed: %v", err))
			if firstErr == nil {
				firstErr = err
			}
		}
		status.IPAssigned = assigned
		route, err := hasLocalRoute(ctx, req.Subnet.CIDR, isIPv4)
		if err != nil {
			status.Errors = append(status.Errors, fmt.Sprintf("route check failed: %v", err))
			if firstErr == nil {
				firstErr = err
			}
		}
		status.RouteExists = route
		statuses = append(statuses, status)
	}
	return statuses, firstErr
}

func interfaceHasSubnetIP(ctx context.Context, iface string, network *net.IPNet) (bool, error) {
	if network.IP.To4() != nil {
		return interfaceHasSubnetIP4(ctx, iface, network)
	}
	return interfaceHasSubnetIP6(ctx, iface, network)
}

func interfaceHasSubnetIP4(ctx context.Context, iface string, network *net.IPNet) (bool, error) {
	output, err := runCommand(ctx, "ip", "-4", "addr", "show", "dev", iface)
	if err != nil {
		return false, err
	}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "inet ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		ipCIDR := fields[1]
		ip, _, err := net.ParseCIDR(ipCIDR)
		if err != nil {
			continue
		}
		ip = ip.To4()
		if ip == nil {
			continue
		}
		if network.Contains(ip) {
			return true, nil
		}
	}
	return false, scanner.Err()
}

func interfaceHasSubnetIP6(ctx context.Context, iface string, network *net.IPNet) (bool, error) {
	output, err := runCommand(ctx, "ip", "-6", "addr", "show", "dev", iface)
	if err != nil {
		return false, err
	}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "inet6 ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		ipCIDR := fields[1]
		ip, _, err := net.ParseCIDR(ipCIDR)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true, nil
		}
	}
	return false, scanner.Err()
}

func hasLocalRoute(ctx context.Context, cidr string, isIPv4 bool) (bool, error) {
	if isIPv4 {
		return hasLocalRoute4(ctx, cidr)
	}
	return hasLocalRoute6(ctx, cidr)
}

func hasLocalRoute4(ctx context.Context, cidr string) (bool, error) {
	output, err := runCommand(ctx, "ip", "-4", "route", "show", "table", "local")
	if err != nil {
		return false, err
	}
	target := fmt.Sprintf("local %s", cidr)
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, target) && strings.Contains(line, " dev lo") {
			return true, nil
		}
	}
	return false, scanner.Err()
}

func hasLocalRoute6(ctx context.Context, cidr string) (bool, error) {
	output, err := runCommand(ctx, "ip", "-6", "route", "show", "table", "local")
	if err != nil {
		return false, err
	}
	target := fmt.Sprintf("local %s", cidr)
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, target) && strings.Contains(line, " dev lo") {
			return true, nil
		}
	}
	return false, scanner.Err()
}

func EnsureMounted(ctx context.Context, requests []Request) ([]Status, error) {
	statuses := make([]Status, 0, len(requests))
	nonLocalStatus, nonLocalErr := readNonLocalBind()
	var firstErr error
	for _, req := range requests {
		isIPv4 := req.Subnet.Network.IP.To4() != nil
		nonLocalSet := false
		if isIPv4 {
			nonLocalSet = nonLocalStatus.IPv4Enabled
		} else {
			nonLocalSet = nonLocalStatus.IPv6Enabled
		}
		status := Status{
			CIDR:         req.Subnet.CIDR,
			Interface:    req.Interface,
			NonLocalBind: nonLocalSet,
		}
		if nonLocalErr != nil {
			status.Errors = append(status.Errors, fmt.Sprintf("nonlocal bind check failed: %v", nonLocalErr))
			if firstErr == nil {
				firstErr = nonLocalErr
			}
		}
		if req.Interface == "" {
			err := fmt.Errorf("no interface configured for subnet %s", req.Subnet.CIDR)
			status.Errors = append(status.Errors, err.Error())
			if firstErr == nil {
				firstErr = err
			}
			statuses = append(statuses, status)
			continue
		}
		assigned, err := interfaceHasSubnetIP(ctx, req.Interface, req.Subnet.Network)
		if err != nil {
			status.Errors = append(status.Errors, fmt.Sprintf("ip check failed: %v", err))
			if firstErr == nil {
				firstErr = err
			}
		}
		status.IPAssigned = assigned
		if !status.IPAssigned {
			ip, ipErr := subnets.DeterministicHost(req.Subnet.Network, req.Subnet.ExcludeHosts)
			if ipErr != nil {
				status.Errors = append(status.Errors, fmt.Sprintf("determine host failed: %v", ipErr))
				if firstErr == nil {
					firstErr = ipErr
				}
			} else {
				status.MountIP = ip
				maskSize, _ := req.Subnet.Network.Mask.Size()
				cidr := fmt.Sprintf("%s/%d", ip.String(), maskSize)
				if _, err := runCommand(ctx, "ip", "addr", "add", cidr, "dev", req.Interface); err != nil {
					status.Errors = append(status.Errors, fmt.Sprintf("ip addr add failed: %v", err))
					if firstErr == nil {
						firstErr = err
					}
				} else {
					status.Actions = append(status.Actions, fmt.Sprintf("ip addr add %s dev %s", cidr, req.Interface))
					recheck, err := interfaceHasSubnetIP(ctx, req.Interface, req.Subnet.Network)
					if err != nil {
						status.Errors = append(status.Errors, fmt.Sprintf("ip recheck failed: %v", err))
						if firstErr == nil {
							firstErr = err
						}
					}
					status.IPAssigned = status.IPAssigned || recheck
				}
			}
		}
		route, err := hasLocalRoute(ctx, req.Subnet.CIDR, isIPv4)
		if err != nil {
			status.Errors = append(status.Errors, fmt.Sprintf("route check failed: %v", err))
			if firstErr == nil {
				firstErr = err
			}
		}
		status.RouteExists = route
		if !status.RouteExists {
			var err error
			var action string
			if isIPv4 {
				_, err = runCommand(ctx, "ip", "-4", "route", "add", "local", req.Subnet.CIDR, "dev", "lo")
				action = fmt.Sprintf("ip -4 route add local %s dev lo", req.Subnet.CIDR)
			} else {
				_, err = runCommand(ctx, "ip", "-6", "route", "add", "local", req.Subnet.CIDR, "dev", "lo")
				action = fmt.Sprintf("ip -6 route add local %s dev lo", req.Subnet.CIDR)
			}
			if err != nil {
				status.Errors = append(status.Errors, fmt.Sprintf("add route failed: %v", err))
				if firstErr == nil {
					firstErr = err
				}
			} else {
				status.Actions = append(status.Actions, action)
				recheck, err := hasLocalRoute(ctx, req.Subnet.CIDR, isIPv4)
				if err != nil {
					status.Errors = append(status.Errors, fmt.Sprintf("route recheck failed: %v", err))
					if firstErr == nil {
						firstErr = err
					}
				}
				status.RouteExists = status.RouteExists || recheck
			}
		}
		if !nonLocalSet {
			if err := setNonLocalBind(); err != nil {
				status.Errors = append(status.Errors, fmt.Sprintf("set nonlocal bind failed: %v", err))
				if firstErr == nil {
					firstErr = err
				}
			} else {
				if isIPv4 {
					status.Actions = append(status.Actions, "set net.ipv4.ip_nonlocal_bind=1")
				} else {
					status.Actions = append(status.Actions, "set net.ipv6.ip_nonlocal_bind=1")
				}
				nonLocalSet = true
				status.NonLocalBind = true
			}
		} else {
			status.NonLocalBind = true
		}
		statuses = append(statuses, status)
	}
	return statuses, firstErr
}

type nonLocalBindStatus struct {
	IPv4Enabled bool
	IPv6Enabled bool
}

func readNonLocalBind() (nonLocalBindStatus, error) {
	var status nonLocalBindStatus
	// Read IPv4
	data4, err := os.ReadFile(filepath.Clean("/proc/sys/net/ipv4/ip_nonlocal_bind"))
	if err == nil {
		value := strings.TrimSpace(string(data4))
		status.IPv4Enabled = value == "1"
	}
	// Read IPv6
	data6, err6 := os.ReadFile(filepath.Clean("/proc/sys/net/ipv6/ip_nonlocal_bind"))
	if err6 == nil {
		value := strings.TrimSpace(string(data6))
		status.IPv6Enabled = value == "1"
	}
	// Return status even if one file is missing (e.g., IPv6 disabled in kernel)
	return status, nil
}

func setNonLocalBind() error {
	// Try to set both, ignore errors for missing files
	err4 := os.WriteFile(filepath.Clean("/proc/sys/net/ipv4/ip_nonlocal_bind"), []byte("1"), 0644)
	err6 := os.WriteFile(filepath.Clean("/proc/sys/net/ipv6/ip_nonlocal_bind"), []byte("1"), 0644)
	// Return error only if both fail
	if err4 != nil && err6 != nil {
		return fmt.Errorf("failed to set nonlocal bind (ipv4: %v, ipv6: %v)", err4, err6)
	}
	return nil
}

var commandMu sync.Mutex

func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	commandMu.Lock()
	defer commandMu.Unlock()
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w (%s)", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}
