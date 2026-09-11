/**
 * Own-position marker icon: a pulsing dot with an optional directional
 * heading cone, rendered as a Leaflet divIcon. Kept separate from
 * aprs-icons.ts (station symbols) — different visual language, and that
 * module is under concurrent edit elsewhere.
 */
import L from 'leaflet';
import type { GpsFix } from './types';

const CONE_LENGTH = 26;
const CONE_HALF_ANGLE_DEG = 30; // 60 deg total width
const CONE_BOX = 56; // px, square SVG viewport the wedge is drawn into

/**
 * Builds the own-position divIcon for one fix. `stale` drives the dimmed /
 * dashed-ring "last known, aging" treatment; the caller is responsible for
 * removing the marker entirely when there is no usable position (mode < 2).
 */
export function createOwnPositionIcon(fix: GpsFix, stale: boolean): L.DivIcon {
	const moving = fix.hasCourse && fix.speedKnots >= 1;

	let coneHtml = '';
	if (moving) {
		const c = CONE_BOX / 2;
		const rad = (CONE_HALF_ANGLE_DEG * Math.PI) / 180;
		const dx = CONE_LENGTH * Math.sin(rad);
		const dy = CONE_LENGTH * Math.cos(rad);
		const left = (c - dx).toFixed(2);
		const right = (c + dx).toFixed(2);
		const tipY = (c - dy).toFixed(2);
		coneHtml = `<svg class="own-pos-cone" width="${CONE_BOX}" height="${CONE_BOX}" viewBox="0 0 ${CONE_BOX} ${CONE_BOX}" style="transform: rotate(${fix.course}deg)">
			<defs>
				<linearGradient id="ownConeGrad" x1="${c}" y1="${c}" x2="${c}" y2="${tipY}" gradientUnits="userSpaceOnUse">
					<stop offset="0" stop-color="rgba(233,69,96,.55)" />
					<stop offset="1" stop-color="rgba(233,69,96,0)" />
				</linearGradient>
			</defs>
			<path d="M ${c},${c} L ${left},${tipY} L ${right},${tipY} Z" fill="url(#ownConeGrad)" />
		</svg>`;
	}

	const pulseHtml = !stale ? '<div class="own-pos-pulse"></div>' : '';
	const staleRingHtml = stale ? '<div class="own-pos-stale-ring"></div>' : '';
	const dotClass = stale ? 'own-pos-dot own-pos-dot--stale' : 'own-pos-dot';

	const html = `<div class="own-pos-wrap">${coneHtml}${pulseHtml}${staleRingHtml}<div class="${dotClass}"></div></div>`;

	return L.divIcon({
		className: 'own-pos-marker',
		html,
		iconSize: [40, 40],
		iconAnchor: [20, 20],
	});
}
