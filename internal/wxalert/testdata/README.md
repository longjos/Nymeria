# wxalert test fixtures

Captured 2026-09-18 from the live NWS API (`https://api.weather.gov`) with a
throwaway contact string, then trimmed by hand/`jq`/`python3` per the rules
below. Two files (`alert_polygon_tor_synthetic.json`, `alert_cancel_tor_synthetic.json`)
are synthetic — no live Tornado Warning was active anywhere in CONUS at
capture time — built from a real TOR's field shapes with invented IDs and
geometry around Grand Rapids, MI. `alert_test_status.json` is likewise
synthetic (NWS does not currently have a live `status: Test` product in the
public feed).

**Tests never regenerate fixtures.** Re-capturing is a conscious, reviewed
diff against this table.

## Trimming rules applied

1. Drop `@context` and every `properties.@id` / `properties.@type`.
2. Truncate `description` to 160 chars + `…` and `instruction` to 120 + `…`
   unless a test asserts on the exact text; `headline` is kept whole.
3. `parameters` kept only for keys the decoder reads: `VTEC`, `NWSheadline`,
   `eventEndingTime`, `flashFloodDamageThreat`, `tornadoDamageThreat`,
   `tornadoDetection`, `maxWindGust`, `maxHailSize`, `eventMotionDescription`.
4. `geocode.UGC` kept whole (it's what the matcher reads); `geocode.SAME`
   and `affectedZones` capped at 5 entries with a sibling
   `"_affectedZonesTrimmed": "N → 5"` note when truncated.
5. Polygons kept whole (the largest here is 10 points).
6. Timestamps (`sent`/`expires`/`ends`) are pinned exactly as captured;
   tests set their clock relative to them, never the other way round.

## Capture commands (re-run only to deliberately refresh a fixture)

```sh
UA='(nymeria-dev, ops@example.org)'

# One live alert feed snapshot.
curl -s -H "User-Agent: $UA" -H 'Accept: application/geo+json' \
  https://api.weather.gov/alerts/active > /tmp/active.json

# Pick one feature by properties.id, then trim it (see trim.jq-equivalent
# rules above — applied by hand/python3 for this capture, not committed as
# a script since jq's the-same-file-twice quoting got unreadable; the rules
# above are the spec a re-capture must follow).
jq '.features[] | select(.properties.id=="urn:oid:...")' /tmp/active.json

# A zone polygon (GeometryCollection case).
curl -s -H "User-Agent: $UA" -H 'Accept: application/geo+json' \
  https://api.weather.gov/zones/forecast/MIZ056 > /tmp/zone.json
jq '{id,type,geometry:{type:"GeometryCollection",
     geometries:[.geometry.geometries[0],.geometry.geometries[1]]
       | map({type:"MultiPolygon", coordinates:[[.coordinates[0][0][0:40]]]})},
     properties:{id:.properties.id,type:.properties.type,name:.properties.name,state:.properties.state}}' \
  /tmp/zone.json > zone_MIZ056.json

# The exact NWS error body for a rejected query parameter (live-verified: a
# 400 with a JSON problem+ body, not a GeoJSON FeatureCollection).
curl -s -H "User-Agent: $UA" -H 'Accept: application/geo+json' \
  'https://api.weather.gov/alerts/active?limit=5' > alert_malformed_not_geojson.json

# A /points lookup, trimmed to the four properties the resolver reads.
curl -s -H "User-Agent: $UA" -H 'Accept: application/geo+json' \
  https://api.weather.gov/points/42.9634,-85.6681 > /tmp/points.json
jq '{properties:{cwa,forecastZone,county,fireWeatherZone}}' /tmp/points.json \
  > points_42.9634_-85.6681.json
```

## Files

| File | Bytes | Source | Used by |
|---|---:|---|---|
| `alert_polygon_ffw_update.json` | 2,948 | Live Flash Flood Warning, NWS Albuquerque NM, `urn:oid:…1c8973854eca…001.1`, 10-pt polygon, `messageType: Update`, one `references[]` entry, VTEC `/O.CON.KABQ.FF.W…/`, `flashFloodDamageThreat: CONSIDERABLE` | decode, geometry |
| `alert_zone_heat_advisory_update.json` | 3,401 | Live Heat Advisory, NWS Nashville TN, `geometry: null`, 33 UGC (`TNZ005`…`TNZ095`), VTEC `/O.EXT.KOHX.HT.Y…/`, `expires` ≠ `ends` | decode (zone-only, the ~85% case), expires-vs-ends |
| `alert_zone_air_quality.json` | 1,772 | Live Air Quality Alert, NWS Memphis TN, `severity/certainty/urgency: Unknown`, `ends: null`, `instruction: null`, no VTEC | decode nulls |
| `alert_zone_sws_null_ends.json` | 2,358 | Live Special Weather Statement, NWS Juneau AK, zone-only, `ends: null` | tier trap, expiry falls back to `expires` |
| `alert_polygon_tor_synthetic.json` | 2,204 | **Synthetic** Tornado Warning, `severity: Extreme`, `urgency: Immediate`, `tornadoDamageThreat: CATASTROPHIC`, 5-pt box around Grand Rapids, UGC `MIC081`,`MIC139`, VTEC `/O.NEW.KGRR.TO.W…/` | classify, interrupt, floor, footprint IN |
| `alert_cancel_tor_synthetic.json` | 1,716 | **Synthetic** Cancel referencing the TOR above | supersession |
| `alert_test_status.json` | 1,199 | **Synthetic** `status: Test`, event `Test` | must-never-reach-operator |
| `alert_exercise_tor.json` (embedded only in the collections below, not read standalone) | — | **Synthetic** `status: Exercise`, event `Tornado Warning` | must-never-reach-operator |
| `alert_malformed_truncated.json` | 500 | `active_collection.json` byte-cut mid-array | decode malformed |
| `alert_malformed_not_geojson.json` | 392 | Live NWS 400 response body from `?limit=5` (rejected query param) | decode malformed |
| `alert_malformed_features_not_array.json` | 93 | Hand-built: `"features": {}` | decode malformed |
| `alert_malformed_feature_missing_properties.json` | 3,327 | Hand-built: one feature with `geometry` but no `properties`, plus the SWS fixture as a second, valid feature | decode partial-degrade |
| `active_collection.json` | 17,755 | FeatureCollection of FFW + Heat Advisory + AQA + SWS + synthetic TOR + synthetic Test + synthetic Exercise (7 features; 2 dropped) | poller, decode |
| `active_collection_v2.json` | 13,951 | Same, FFW replaced by a newer Update (new id, `sent` +10 min) and the Heat Advisory silently removed | poller change detection, silent drop |
| `zone_MIZ056.json` | 5,139 | Live `GeometryCollection` zone (Ottawa Co. MI forecast zone), trimmed to 2 sub-geometries ≤ 40 points each | zone cache GeometryCollection decode |
| `zone_MIC081.json` | 443 | **Synthetic** `Polygon`, 6 points, Kent County-ish box around Grand Rapids | zone cache hit/miss/prefetch |
| `zone_corrupt.json` | 83 | Hand-truncated mid-object | corrupt-file recovery |
| `points_42.9634_-85.6681.json` | 241 | Live `/points` response, trimmed | UGC resolver (`ResolvePoint`) |
| `count_sample.json` | 628 | Live `/alerts/active/count` response, trimmed | reference only (poller tests build their own count bodies) |

All fixtures verified byte-parseable (or deliberately not, for the
`alert_malformed_*` set) as of capture. `zone_MIZ056.json`'s real upstream
payload is ~420 KB; this trimmed copy keeps the `GeometryCollection` shape
class (member geometries are themselves `MultiPolygon`, a real NWS quirk)
at 2 members / 40 points each.
