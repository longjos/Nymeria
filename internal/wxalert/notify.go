package wxalert

import (
	"fmt"
	"strings"
)

// DefaultInterruptEvents is the seed allowlist (user decision 1): NCS-
// editable per net, but no edit may drop an Extreme+Immediate+in-footprint
// alert below a toast (the hard floor in Classify, step 6, is unconditional
// and does not consult this list at all). Order is display order.
var DefaultInterruptEvents = []string{
	"Tornado Warning", "Flash Flood Emergency", "Severe Thunderstorm Warning", "Flash Flood Warning",
	"Extreme Wind Warning", "Ice Storm Warning", "Blizzard Warning",
}

// Policy is the effective (config + per-net) notification policy.
type Policy struct {
	InterruptEvents []string
	WatchNotify     NotifyClass // toast (default) | badge
	AdvisoryNotify  NotifyClass // badge (default) | panel
	StatementNotify NotifyClass // panel (default) | badge
	MuteAdvisories  bool
}

// DefaultPolicy is the out-of-the-box policy: the seed allowlist, and the
// quietest class each tier is allowed to be lowered to.
func DefaultPolicy() Policy {
	return Policy{
		InterruptEvents: append([]string{}, DefaultInterruptEvents...),
		WatchNotify:     NotifyToast,
		AdvisoryNotify:  NotifyBadge,
		StatementNotify: NotifyPanel,
	}
}

// Input is everything Classify needs about one alert version to decide how
// loudly to notify.
type Input struct {
	Event          string // the alert's plain Event; only needed when it differs from EffectiveEvent
	EffectiveEvent string
	Tier           Tier
	Severity       Severity
	Urgency        Urgency
	Proximity      Proximity // "in" | "near" | "far"
	IsTest         bool
	Active         bool        // false for expired/cancelled/dropped
	PrevClass      NotifyClass // the class the alert this one replaces last got
	IsUpdate       bool        // this version references one already shown
	Escalated      bool        // severity/tier rank up, or proximity went near->in
	NetAcked       bool        // NCS acknowledged the previous version for the net
}

// Result is what Classify decided.
type Result struct {
	Class   NotifyClass
	Reason  string // "new" | "update" | "escalated" | "ended" | "none"
	Floored bool   // true when the hard floor (step 6) raised the class
}

// InAllowlist reports whether event is in list, case-insensitive and
// trimmed. Exact match only — the seed list names exactly what it names.
func InAllowlist(event string, list []string) bool {
	event = strings.TrimSpace(event)
	for _, e := range list {
		if strings.EqualFold(strings.TrimSpace(e), event) {
			return true
		}
	}
	return false
}

// ValidateInterruptEvents canonicalizes and validates an NCS-edited
// allowlist: every entry must be a recognized event name at warning tier.
// Duplicates (by case) collapse to one canonical entry; order is preserved.
// A nil/empty input returns a non-nil empty slice.
func ValidateInterruptEvents(events []string) ([]string, error) {
	out := make([]string, 0, len(events))
	seen := map[string]bool{}
	for _, raw := range events {
		canon, ok := CanonicalEventName(raw)
		if !ok {
			return nil, fmt.Errorf("unknown event: %s", strings.TrimSpace(raw))
		}
		tier, _ := TierForEvent(canon)
		if tier != TierWarning {
			return nil, fmt.Errorf("interrupt list may only contain warning-tier events: %s", canon)
		}
		key := strings.ToLower(canon)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, canon)
	}
	return out, nil
}

// Classify decides the notification class for one version of an alert,
// evaluated in the fixed order documented in BUILD-PLAN.md §4.4:
//
//  1. Test status / Far proximity -> panel, never broadcast.
//  2. Non-active (expired/cancelled/dropped) -> the "all clear" rule.
//  3. Base class from the tier/allowlist/severity matrix.
//  4. Update rule: a non-escalated update goes one step quieter.
//  5. A new or escalated IN warning is never quieter than toast.
//  6. The hard floor: Extreme+Immediate+IN is never quieter than toast,
//     unconditionally and last, so nothing above it can turn it off.
func Classify(in Input, p Policy) Result {
	if in.IsTest {
		return Result{Class: NotifyPanel, Reason: "none"}
	}
	if in.Proximity != ProximityIn && in.Proximity != ProximityNear {
		return Result{Class: NotifyPanel, Reason: "none"}
	}

	if !in.Active {
		if NotifyRank(in.PrevClass) >= NotifyRank(NotifyToast) &&
			in.Proximity == ProximityIn && TierRank(in.Tier) >= TierRank(TierWatch) {
			return Result{Class: NotifyToast, Reason: "ended"}
		}
		return Result{Class: NotifyPanel, Reason: "ended"}
	}

	class := baseClass(in, p)

	reason := "new"
	switch {
	case in.Escalated:
		reason = "escalated"
	case in.IsUpdate:
		reason = "update"
		class = StepQuieter(class)
		if in.NetAcked {
			class = Quieter(class, NotifyBadge)
		}
		if in.Tier == TierWatch {
			class = Quieter(class, NotifyBadge)
		}
	}

	if (reason == "new" || reason == "escalated") && in.Tier == TierWarning && in.Proximity == ProximityIn {
		class = Louder(class, NotifyToast)
	}

	// CAP urgency "Past" means the event already happened or was never going
	// to: nothing at this point is worth more than a panel entry, even a
	// warning-tier alert that step 5 would otherwise have floored to toast.
	// (This can never fight the hard floor below: an alert's urgency is a
	// single value, so Past and Immediate never hold at once.)
	if in.Urgency == UrgencyPast {
		class = NotifyPanel
	}

	floored := false
	if in.Proximity == ProximityIn && in.Severity == SeverityExtreme && in.Urgency == UrgencyImmediate &&
		NotifyRank(class) < NotifyRank(NotifyToast) {
		class = NotifyToast
		floored = true
	}

	return Result{Class: class, Reason: reason, Floored: floored}
}

func baseClass(in Input, p Policy) NotifyClass {
	// An emergency-derived alert (EffectiveEvent "Flash Flood Emergency" /
	// "Tornado Emergency") is still the warning it was built from underneath,
	// so an allowlist entry naming the plain event also matches it.
	allowlisted := InAllowlist(in.EffectiveEvent, p.InterruptEvents) ||
		(in.Event != "" && InAllowlist(in.Event, p.InterruptEvents))
	eligibleForInterrupt := in.Tier == TierWarning &&
		in.Urgency != UrgencyPast && in.Urgency != UrgencyUnknown &&
		allowlisted

	var class NotifyClass
	switch {
	case eligibleForInterrupt:
		if in.Proximity == ProximityIn {
			class = NotifyInterrupt
		} else {
			class = NotifyToast
		}
	case in.Tier == TierWarning:
		if in.Severity == SeverityExtreme || in.Severity == SeveritySevere {
			if in.Urgency == UrgencyImmediate {
				class = NotifyToast // both IN and NEAR
				break
			}
		}
		if in.Proximity == ProximityIn {
			class = NotifyToast
		} else {
			class = NotifyBadge
		}
	case in.Tier == TierWatch:
		if in.Proximity == ProximityIn {
			class = p.WatchNotify
		} else {
			class = NotifyBadge
		}
	case in.Tier == TierAdvisory:
		if in.Proximity == ProximityIn {
			class = p.AdvisoryNotify
		} else {
			class = NotifyPanel
		}
	case in.Tier == TierStatement:
		if in.Proximity == ProximityIn {
			class = p.StatementNotify
		} else {
			class = NotifyPanel
		}
	default:
		class = NotifyPanel
	}

	if p.MuteAdvisories && (in.Tier == TierAdvisory || in.Tier == TierStatement) {
		class = NotifyPanel
	}
	return class
}
