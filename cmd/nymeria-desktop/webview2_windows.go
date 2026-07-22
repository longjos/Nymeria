//go:build windows

package main

import (
	"golang.org/x/sys/windows/registry"

	"github.com/narvel/nymeria/internal/app"
)

// webview2Available reports whether the WebView2 Evergreen runtime is
// installed, using the registry locations Microsoft documents for runtime
// detection: per-machine (32-bit view, then native), then per-user.
func webview2Available() bool {
	const clientKey = `Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`
	checks := []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\` + clientKey},
		{registry.LOCAL_MACHINE, `SOFTWARE\` + clientKey},
		{registry.CURRENT_USER, `Software\` + clientKey},
	}
	for _, c := range checks {
		k, err := registry.OpenKey(c.root, c.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		pv, _, err := k.GetStringValue("pv")
		k.Close()
		if err == nil && app.WebView2VersionUsable(pv) {
			return true
		}
	}
	return false
}
