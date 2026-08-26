// Package timeutil holds the unit's local-time helpers.
package timeutil

import "time"

// SGT is Singapore time (UTC+8). A fixed zone is used rather than
// LoadLocation so the binary does not depend on a timezone database being
// present in the container image.
var SGT = time.FixedZone("SGT", 8*60*60)

// NowSGT returns the current instant expressed in Singapore time.
func NowSGT() time.Time {
	return time.Now().In(SGT)
}
