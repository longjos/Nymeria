package app

import (
	"context"
	"fmt"
	"log"

	"github.com/narvel/nymeria/internal/config"
	"github.com/narvel/nymeria/internal/transport"
	"github.com/narvel/nymeria/internal/transport/aprsis"
	"github.com/narvel/nymeria/internal/transport/kisstcp"
	"github.com/narvel/nymeria/internal/transport/serial"
)

// reconcileTransports diffs old and new transport configs and updates the transport manager.
func reconcileTransports(ctx context.Context, old, newCfg config.Config, tm *transport.Manager) {
	oldMap := buildTransportMap(old.Transports)
	newMap := buildTransportMap(newCfg.Transports)

	// Remove transports that are gone
	for id := range oldMap {
		if _, exists := newMap[id]; !exists {
			if err := tm.Remove(id); err != nil {
				log.Printf("[config] failed to remove transport %s: %v", id, err)
			} else {
				log.Printf("[config] transport removed: %s", id)
			}
		}
	}

	// Add new transports and reconfigure changed ones
	for id, newTC := range newMap {
		oldTC, existed := oldMap[id]

		if !existed {
			// New transport
			t := createTransport(newTC, newCfg.Station.Callsign)
			if t == nil {
				continue
			}
			tm.AddNamed(id, t, newTC.Name)
			if err := tm.ConnectOne(ctx, id); err != nil {
				log.Printf("[config] failed to connect transport %s: %v", id, err)
			} else {
				log.Printf("[config] transport added: %s", id)
			}
			continue
		}

		if !transportConfigEqual(oldTC, newTC) {
			// Config changed — remove old, add new
			if err := tm.Remove(id); err != nil {
				log.Printf("[config] failed to remove transport %s for reconfigure: %v", id, err)
				continue
			}
			t := createTransport(newTC, newCfg.Station.Callsign)
			if t == nil {
				continue
			}
			tm.AddNamed(id, t, newTC.Name)
			if err := tm.ConnectOne(ctx, id); err != nil {
				log.Printf("[config] failed to reconnect transport %s: %v", id, err)
			} else {
				log.Printf("[config] transport reconfigured: %s", id)
			}
		}
	}
}

// buildTransportMap creates a stable ID → config map from a transport config slice.
func buildTransportMap(configs []transport.TransportConfig) map[string]transport.TransportConfig {
	m := make(map[string]transport.TransportConfig, len(configs))
	counts := make(map[string]int)
	for _, tc := range configs {
		n := counts[tc.Type]
		counts[tc.Type]++
		id := fmt.Sprintf("%s-%d", tc.Type, n)
		m[id] = tc
	}
	return m
}

// createTransport instantiates a transport from config, returning nil for unknown types.
func createTransport(tc transport.TransportConfig, stationCallsign string) transport.Transport {
	switch tc.Type {
	case "aprsis":
		if tc.Callsign == "" {
			tc.Callsign = stationCallsign
		}
		return aprsis.New(tc)
	case "kisstcp":
		return kisstcp.New(tc)
	case "serial":
		return serial.New(tc)
	default:
		log.Printf("[config] unknown transport type %q, skipping", tc.Type)
		return nil
	}
}

// transportConfigEqual returns true if two transport configs are identical.
func transportConfigEqual(a, b transport.TransportConfig) bool {
	return a.Type == b.Type &&
		a.Name == b.Name &&
		a.Host == b.Host &&
		a.Port == b.Port &&
		a.Device == b.Device &&
		a.Baud == b.Baud &&
		a.Filter == b.Filter &&
		a.Callsign == b.Callsign &&
		a.Passcode == b.Passcode
}
