//go:build !linux

package clutch

import "log"

// StartSniffer is not available on non-Linux platforms.
func StartSniffer() error {
	log.Printf("sniffer: not available on this platform, skipping")
	return nil
}
