package app

// WebView2VersionUsable reports whether a WebView2 Evergreen runtime version
// string (the "pv" registry value) denotes an installed runtime. Microsoft
// documents an absent value, an empty string, and "0.0.0.0" as not installed.
func WebView2VersionUsable(pv string) bool {
	return pv != "" && pv != "0.0.0.0"
}
