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
	nonLocal, nonLocalErr := readNonLocalBind()
	for _, req := range requests {
		status := Status{
			CIDR:         req.Subnet.CIDR,
			Interface:    req.Interface,
			NonLocalBind: nonLocal,
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
		}
		status.IPAssigned = assigned
		route, err := hasLocalRoute(ctx, req.Subnet.CIDR)
		if err != nil {
			status.Errors = append(status.Errors, fmt.Sprintf("route check failed: %v", err))
		}
		status.RouteExists = route
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func EnsureMounted(ctx context.Context, requests []Request) ([]Status, error) {
	statuses := make([]Status, 0, len(requests))
	nonLocal, nonLocalErr := readNonLocalBind()
	nonLocalSet := nonLocal
	for _, req := range requests {
		status := Status{
			CIDR:         req.Subnet.CIDR,
			Interface:    req.Interface,
			NonLocalBind: nonLocalSet,
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
		}
		status.IPAssigned = assigned
		if !status.IPAssigned {
			ip, ipErr := subnets.DeterministicHost(req.Subnet.Network, req.Subnet.ExcludeHosts)
			if ipErr != nil {
				status.Errors = append(status.Errors, fmt.Sprintf("determine host failed: %v", ipErr))
			} else {
				status.MountIP = ip
				maskSize, _ := req.Subnet.Network.Mask.Size()
				cidr := fmt.Sprintf("%s/%d", ip.String(), maskSize)
				if _, err := runCommand(ctx, "ip", "addr", "add", cidr, "dev", req.Interface); err != nil {
					status.Errors = append(status.Errors, fmt.Sprintf("ip addr add failed: %v", err))
				} else {
					status.Actions = append(status.Actions, fmt.Sprintf("ip addr add %s dev %s", cidr, req.Interface))
					recheck, err := interfaceHasSubnetIP(ctx, req.Interface, req.Subnet.Network)
					if err != nil {
						status.Errors = append(status.Errors, fmt.Sprintf("ip recheck failed: %v", err))
					}
					status.IPAssigned = status.IPAssigned || recheck
				}
			}
		}
		route, err := hasLocalRoute(ctx, req.Subnet.CIDR)
		if err != nil {
			status.Errors = append(status.Errors, fmt.Sprintf("route check failed: %v", err))
		}
		status.RouteExists = route
		if !status.RouteExists {
			if _, err := runCommand(ctx, "ip", "route", "add", "local", req.Subnet.CIDR, "dev", "lo"); err != nil {
				status.Errors = append(status.Errors, fmt.Sprintf("add route failed: %v", err))
			} else {
				status.Actions = append(status.Actions, fmt.Sprintf("ip route add local %s dev lo", req.Subnet.CIDR))
				recheck, err := hasLocalRoute(ctx, req.Subnet.CIDR)
				if err != nil {
					status.Errors = append(status.Errors, fmt.Sprintf("route recheck failed: %v", err))
				}
				status.RouteExists = status.RouteExists || recheck
			}
		}
		if !nonLocalSet {
			if err := setNonLocalBind(); err != nil {
				status.Errors = append(status.Errors, fmt.Sprintf("set nonlocal bind failed: %v", err))
			} else {
				status.Actions = append(status.Actions, "set net.ipv4.ip_nonlocal_bind=1")
				nonLocalSet = true
				status.NonLocalBind = true
			}
		} else {
			status.NonLocalBind = true
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func interfaceHasSubnetIP(ctx context.Context, iface string, network *net.IPNet) (bool, error) {
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

func hasLocalRoute(ctx context.Context, cidr string) (bool, error) {
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

func readNonLocalBind() (bool, error) {
	data, err := os.ReadFile(filepath.Clean("/proc/sys/net/ipv4/ip_nonlocal_bind"))
	if err != nil {
		return false, err
	}
	value := strings.TrimSpace(string(data))
	return value == "1", nil
}

func setNonLocalBind() error {
	return os.WriteFile(filepath.Clean("/proc/sys/net/ipv4/ip_nonlocal_bind"), []byte("1"), 0644)
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
