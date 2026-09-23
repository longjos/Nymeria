# SAG dock — use-case scenarios

The SAG dock (`web/src/lib/components/ride/SagDock.svelte`) is the list beside
the map on desktop/tablet and the bottom sheet's `sag` mode on a phone. It
lists requests well, but every state change after "Find a driver" has to be
done on the SAG board in the side panel. On a ride, SAG traffic arrives as
**radio calls from drivers**, many per minute, and the operator's hands and
eyes are on the map. These scenarios define what the dock has to do.

Status model (server-enforced, `internal/ride/status.go`): a leg moves
`dispatched → enroute → onscene → loaded → delivered`, forward only, with
forward skips allowed ("SAG 2 has both riders" goes dispatched → loaded).
`loaded` is reached by the load verb (which slots, bike disposition); `delivered`
by the deliver verb. A leg can be released with a reason. A slot can resolve
to an exception (e.g. `self_resolved`, "fixed own flat, rode on"). Nothing moves
backwards, so a mis-tap cannot be undone by the operator.

Today, per scenario: ✗ = not possible from the dock, ◐ = possible but slow.

## S1 — "SAG 2, on scene at the Eakin rest stop" ✗
The operator must move SAG 2's leg from en route to on scene **without leaving
the map**: find SAG 2 by the name the driver just said, one deliberate action,
visible confirmation. The request card in the dock shows no leg status and no
actions once a driver is assigned.

## S2 — "SAG 2 has both riders and both bikes, heading to Flat Creek" ✗
Load: which riders (usually all waiting on this leg) and what happens to each
bike (usually with the rider). The common answer must be one confirm; the
uncommon (one bike left behind, one rider declined) must still be reachable
without the side panel.

## S3 — "SAG 2 dropped them at Flat Creek" ✗
Deliver. One confirm, naming the dropoff. The request leaves the dock's active
groups once complete.

## S4 — "SAG 2, the rider fixed the flat and rode on" ✗
Resolve the slot as self-resolved, which frees the vehicle. Needs to be
reachable from the same place as S1–S3 but must not sit next to the primary
advance action where it can be hit by mistake.

## S5 — "Where is SAG 2?" / "What's going on at mile 30?" ◐
- Clicking a vehicle zooms the map to it (its last fix; a stale fix is shown
  as stale, never as current).
- Clicking a request zooms to the **extent of that job**: pickup, the assigned
  vehicle(s) and the dropoff, with the leg line — padded clear of the dock and
  strip.
- A "zoom to all SAG" control fits every open pickup and every SAG vehicle.
Today a card click selects a request but does not fit the map to it.

## S6 — "Who's free?" — driver-centric view ✗
Radio calls are keyed by the DRIVER, not the request. The operator needs a
vehicle view: each SAG unit, its current job and leg status, seats/racks free,
position age — and the same advance actions (S1–S3) from the vehicle row. A
vehicle with two jobs (second pickup after a rest stop) shows both, each
advanced independently.

## S7 — Mis-tap safety
Advances are irreversible. A wrong tap in a moving vehicle must be hard to make
and obvious if made: the action names the target state and the vehicle
("SAG 2 → On scene"), and skipping forward (en route → loaded) is a deliberate,
separate choice — never the default button. Every commit gets visible
feedback; a server refusal says why.

## S8 — Keyboard at a desk
The dock is one tab stop with roving focus today (p / m / i per card). The
operator should be able to: jump to a vehicle by typing its tactical call,
advance with a single named key, and zoom with another — all announced to a
screen reader.

## S9 — Every surface
Desktop dock (260px), tablet collapsed rail (44px) and expanded overlay, and
the phone bottom sheet's `sag` mode. 44px touch targets on touch; nothing
depends on hover.

## S10 — Busy ride
Eight vehicles, fifteen open requests, three unplaceable. The dock must still
lead with what is blocked on the operator (the existing ordering), and the new
actions must not double the height of every card.
