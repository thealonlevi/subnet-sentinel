package mount

import (
	"context"
	"net"

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
	_ = ctx
	statuses := make([]Status, 0, len(requests))
	for _, req := range requests {
		statuses = append(statuses, Status{
			CIDR:      req.Subnet.CIDR,
			Interface: req.Interface,
			Errors:    []string{"mount checks are disabled in this version"},
		})
	}
	return statuses, nil
}

func EnsureMounted(ctx context.Context, requests []Request) ([]Status, error) {
	_ = ctx
	statuses := make([]Status, 0, len(requests))
	for _, req := range requests {
		statuses = append(statuses, Status{
			CIDR:      req.Subnet.CIDR,
			Interface: req.Interface,
			Errors:    []string{"mount operations are disabled in this version"},
		})
	}
	return statuses, nil
}
