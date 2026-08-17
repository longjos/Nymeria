package kisstcp

import (
	"context"
	"net"
	"strings"

	"github.com/grandcat/zeroconf"
)

func browseMDNSDefault() ([]rawTNC, error) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return nil, err
	}

	entries := make(chan *zeroconf.ServiceEntry, 16)
	ctx, cancel := context.WithTimeout(context.Background(), mdnsBrowseTimeout)
	defer cancel()

	if err := resolver.Browse(ctx, kissServiceType, kissServiceDom, entries); err != nil {
		return nil, err
	}

	var out []rawTNC
	seen := map[string]bool{}
	for {
		select {
		case <-ctx.Done():
			return out, nil
		case e, ok := <-entries:
			if !ok {
				return out, nil
			}
			if e == nil {
				continue
			}
			host := pickTNCHost(e)
			if host == "" {
				continue
			}
			key := addrKey(host, e.Port)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, rawTNC{
				Instance:  e.Instance,
				Host:      host,
				Port:      e.Port,
				PortsNote: txtValue(e.Text, "pn"),
			})
		}
	}
}

func pickTNCHost(e *zeroconf.ServiceEntry) string {
	for _, ip := range e.AddrIPv4 {
		if ip == nil || ip.IsUnspecified() {
			continue
		}
		return ip.String()
	}
	for _, ip := range e.AddrIPv6 {
		if ip == nil || ip.IsUnspecified() || ip.IsLinkLocalUnicast() {
			continue
		}
		return ip.String()
	}
	h := strings.TrimSpace(e.HostName)
	h = strings.TrimSuffix(h, ".")
	if h != "" {
		// Prefer resolving A records; HostName.local often works via mDNS.
		if ips, err := net.LookupIP(h); err == nil {
			for _, ip := range ips {
				if ip.To4() != nil && !ip.IsUnspecified() {
					return ip.String()
				}
			}
		}
		return h
	}
	return ""
}

func txtValue(txt []string, key string) string {
	prefix := key + "="
	for _, t := range txt {
		if strings.HasPrefix(t, prefix) {
			return strings.TrimPrefix(t, prefix)
		}
	}
	return ""
}
