/**
 * APRS Symbol → Lucide icon mapping for map markers.
 *
 * SVG inner elements extracted from lucide-static v0.563.0.
 * No runtime dependency on lucide — paths are embedded directly.
 */
import type { APRSSymbol } from './types';

// Lucide icon inner SVG elements (24x24 viewBox, stroke-based)
const I = {
	shieldAlert: '<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z" /><path d="M12 8v4" /><path d="M12 16h.01" />',
	radio: '<path d="M16.247 7.761a6 6 0 0 1 0 8.478" /><path d="M19.075 4.933a10 10 0 0 1 0 14.134" /><path d="M4.925 19.067a10 10 0 0 1 0-14.134" /><path d="M7.753 16.239a6 6 0 0 1 0-8.478" /><circle cx="12" cy="12" r="2" />',
	phone: '<path d="M13.832 16.568a1 1 0 0 0 1.213-.303l.355-.465A2 2 0 0 1 17 15h3a2 2 0 0 1 2 2v3a2 2 0 0 1-2 2A18 18 0 0 1 2 4a2 2 0 0 1 2-2h3a2 2 0 0 1 2 2v3a2 2 0 0 1-.8 1.6l-.468.351a1 1 0 0 0-.292 1.233 14 14 0 0 0 6.392 6.384" />',
	router: '<rect width="20" height="8" x="2" y="14" rx="2" /><path d="M6.01 18H6" /><path d="M10.01 18H10" /><path d="M15 10v4" /><path d="M17.84 7.17a4 4 0 0 0-5.66 0" /><path d="M20.66 4.34a8 8 0 0 0-11.31 0" />',
	plane: '<path d="M17.8 19.2 16 11l3.5-3.5C21 6 21.5 4 21 3c-1-.5-3 0-4.5 1.5L13 8 4.8 6.2c-.5-.1-.9.1-1.1.5l-.3.5c-.2.5-.1 1 .3 1.3L9 12l-2 3H4l-1 1 3 2 2 3 1-1v-3l3-2 3.5 5.3c.3.4.8.5 1.3.3l.5-.2c.4-.3.6-.7.5-1.2z" />',
	cloud: '<path d="M17.5 19H9a7 7 0 1 1 6.71-9h1.79a4.5 4.5 0 1 1 0 9Z" />',
	satellite: '<path d="m13.5 6.5-3.148-3.148a1.205 1.205 0 0 0-1.704 0L6.352 5.648a1.205 1.205 0 0 0 0 1.704L9.5 10.5" /><path d="M16.5 7.5 19 5" /><path d="m17.5 10.5 3.148 3.148a1.205 1.205 0 0 1 0 1.704l-2.296 2.296a1.205 1.205 0 0 1-1.704 0L13.5 14.5" /><path d="M9 21a6 6 0 0 0-6-6" /><path d="M9.352 10.648a1.205 1.205 0 0 0 0 1.704l2.296 2.296a1.205 1.205 0 0 0 1.704 0l4.296-4.296a1.205 1.205 0 0 0 0-1.704l-2.296-2.296a1.205 1.205 0 0 0-1.704 0z" />',
	snowflake: '<path d="m10 20-1.25-2.5L6 18" /><path d="M10 4 8.75 6.5 6 6" /><path d="m14 20 1.25-2.5L18 18" /><path d="m14 4 1.25 2.5L18 6" /><path d="m17 21-3-6h-4" /><path d="m17 3-3 6 1.5 3" /><path d="M2 12h6.5L10 9" /><path d="m20 10-1.5 2 1.5 2" /><path d="M22 12h-6.5L14 15" /><path d="m4 10 1.5 2L4 14" /><path d="m7 21 3-6-1.5-3" /><path d="m7 3 3 6h4" />',
	cross: '<path d="M4 9a2 2 0 0 0-2 2v2a2 2 0 0 0 2 2h4a1 1 0 0 1 1 1v4a2 2 0 0 0 2 2h2a2 2 0 0 0 2-2v-4a1 1 0 0 1 1-1h4a2 2 0 0 0 2-2v-2a2 2 0 0 0-2-2h-4a1 1 0 0 1-1-1V4a2 2 0 0 0-2-2h-2a2 2 0 0 0-2 2v4a1 1 0 0 1-1 1z" />',
	compass: '<path d="m16.24 7.76-1.804 5.411a2 2 0 0 1-1.265 1.265L7.76 16.24l1.804-5.411a2 2 0 0 1 1.265-1.265z" /><circle cx="12" cy="12" r="10" />',
	home: '<path d="M15 21v-8a1 1 0 0 0-1-1h-4a1 1 0 0 0-1 1v8" /><path d="M3 10a2 2 0 0 1 .709-1.528l7-6a2 2 0 0 1 2.582 0l7 6A2 2 0 0 1 21 10v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />',
	x: '<path d="M18 6 6 18" /><path d="m6 6 12 12" />',
	circleDot: '<circle cx="12" cy="12" r="10" /><circle cx="12" cy="12" r="1" />',
	hash: '<line x1="4" x2="20" y1="9" y2="9" /><line x1="4" x2="20" y1="15" y2="15" /><line x1="10" x2="8" y1="3" y2="21" /><line x1="16" x2="14" y1="3" y2="21" />',
	flame: '<path d="M12 3q1 4 4 6.5t3 5.5a1 1 0 0 1-14 0 5 5 0 0 1 1-3 1 1 0 0 0 5 0c0-2-1.5-3-1.5-5q0-2 2.5-4" />',
	tent: '<path d="M3.5 21 14 3" /><path d="M20.5 21 10 3" /><path d="M15.5 21 12 15l-3.5 6" /><path d="M2 21h20" />',
	bike: '<circle cx="18.5" cy="17.5" r="3.5" /><circle cx="5.5" cy="17.5" r="3.5" /><circle cx="15" cy="5" r="1" /><path d="M12 17.5V14l-3-3 4-3 2 3h2" />',
	train: '<rect width="16" height="16" x="4" y="3" rx="2" /><path d="M4 11h16" /><path d="M12 3v8" /><path d="m8 19-2 3" /><path d="m18 22-2-3" /><path d="M8 15h.01" /><path d="M16 15h.01" />',
	car: '<path d="M19 17h2c.6 0 1-.4 1-1v-3c0-.9-.7-1.7-1.5-1.9C18.7 10.6 16 10 16 10s-1.3-1.4-2.2-2.3c-.5-.4-1.1-.7-1.8-.7H5c-.6 0-1.1.4-1.4.9l-1.4 2.9A3.7 3.7 0 0 0 2 12v4c0 .6.4 1 1 1h2" /><circle cx="7" cy="17" r="2" /><path d="M9 17h6" /><circle cx="17" cy="17" r="2" />',
	server: '<rect width="20" height="8" x="2" y="2" rx="2" ry="2" /><rect width="20" height="8" x="2" y="14" rx="2" ry="2" /><line x1="6" x2="6.01" y1="6" y2="6" /><line x1="6" x2="6.01" y1="18" y2="18" />',
	heartPulse: '<path d="M2 9.5a5.5 5.5 0 0 1 9.591-3.676.56.56 0 0 0 .818 0A5.49 5.49 0 0 1 22 9.5c0 2.29-1.5 4-3 5.5l-5.492 5.313a2 2 0 0 1-3 .019L5 15c-1.5-1.5-3-3.2-3-5.5" /><path d="M3.22 13H9.5l.5-1 2 4.5 2-7 1.5 3.5h5.27" />',
	messageSquare: '<path d="M22 17a2 2 0 0 1-2 2H6.828a2 2 0 0 0-1.414.586l-2.202 2.202A.71.71 0 0 1 2 21.286V5a2 2 0 0 1 2-2h16a2 2 0 0 1 2 2z" />',
	ship: '<path d="M12 10.189V14" /><path d="M12 2v3" /><path d="M19 13V7a2 2 0 0 0-2-2H7a2 2 0 0 0-2 2v6" /><path d="M19.38 20A11.6 11.6 0 0 0 21 14l-8.188-3.639a2 2 0 0 0-1.624 0L3 14a11.6 11.6 0 0 0 2.81 7.76" /><path d="M2 21c.6.5 1.2 1 2.5 1 2.5 0 2.5-2 5-2 1.3 0 1.9.5 2.5 1s1.2 1 2.5 1c2.5 0 2.5-2 5-2 1.3 0 1.9.5 2.5 1" />',
	eye: '<path d="M2.062 12.348a1 1 0 0 1 0-.696 10.75 10.75 0 0 1 19.876 0 1 1 0 0 1 0 .696 10.75 10.75 0 0 1-19.876 0" /><circle cx="12" cy="12" r="3" />',
	hotel: '<path d="M10 22v-6.57" /><path d="M12 11h.01" /><path d="M12 7h.01" /><path d="M14 15.43V22" /><path d="M15 16a5 5 0 0 0-6 0" /><path d="M16 11h.01" /><path d="M16 7h.01" /><path d="M8 11h.01" /><path d="M8 7h.01" /><rect x="4" y="2" width="16" height="20" rx="2" />',
	globe: '<circle cx="12" cy="12" r="10" /><path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20" /><path d="M2 12h20" />',
	graduationCap: '<path d="M21.42 10.922a1 1 0 0 0-.019-1.838L12.83 5.18a2 2 0 0 0-1.66 0L2.6 9.08a1 1 0 0 0 0 1.832l8.57 3.908a2 2 0 0 0 1.66 0z" /><path d="M22 10v6" /><path d="M6 12.5V16a6 3 0 0 0 12 0v-3.5" />',
	laptop: '<path d="M18 5a2 2 0 0 1 2 2v8.526a2 2 0 0 0 .212.897l1.068 2.127a1 1 0 0 1-.9 1.45H3.62a1 1 0 0 1-.9-1.45l1.068-2.127A2 2 0 0 0 4 15.526V7a2 2 0 0 1 2-2z" /><path d="M20.054 15.987H3.946" />',
	rocket: '<path d="M4.5 16.5c-1.5 1.26-2 5-2 5s3.74-.5 5-2c.71-.84.7-2.13-.09-2.91a2.18 2.18 0 0 0-2.91-.09z" /><path d="m12 15-3-3a22 22 0 0 1 2-3.95A12.88 12.88 0 0 1 22 2c0 2.72-.78 7.5-6 11a22.35 22.35 0 0 1-4 2z" /><path d="M9 12H4s.55-3.03 2-4c1.62-1.08 5 0 5 0" /><path d="M12 15v5s3.03-.55 4-2c1.08-1.62 0-5 0-5" />',
	tv: '<path d="m17 2-5 5-5-5" /><rect width="20" height="15" x="2" y="7" rx="2" />',
	bus: '<path d="M8 6v6" /><path d="M15 6v6" /><path d="M2 12h19.6" /><path d="M18 18h3s.5-1.7.8-2.8c.1-.4.2-.8.2-1.2 0-.4-.1-.8-.2-1.2l-1.4-5C20.1 6.8 19.1 6 18 6H4a2 2 0 0 0-2 2v10h3" /><circle cx="7" cy="18" r="2" /><path d="M9 18h5" /><circle cx="16" cy="18" r="2" />',
	cloudSun: '<path d="M12 2v2" /><path d="m4.93 4.93 1.41 1.41" /><path d="M20 12h2" /><path d="m19.07 4.93-1.41 1.41" /><path d="M15.947 12.65a4 4 0 0 0-5.925-4.128" /><path d="M13 22H7a5 5 0 1 1 4.9-6H13a3 3 0 0 1 0 6Z" />',
	sailboat: '<path d="M10 2v15" /><path d="M7 22a4 4 0 0 1-4-4 1 1 0 0 1 1-1h16a1 1 0 0 1 1 1 4 4 0 0 1-4 4z" /><path d="M9.159 2.46a1 1 0 0 1 1.521-.193l9.977 8.98A1 1 0 0 1 20 13H4a1 1 0 0 1-.824-1.567z" />',
	triangle: '<path d="M13.73 4a2 2 0 0 0-3.46 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z" />',
	thermometer: '<path d="M14 4v10.54a4 4 0 1 1-4 0V4a2 2 0 0 1 4 0Z" />',
	satelliteDish: '<path d="M4 10a7.31 7.31 0 0 0 10 10Z" /><path d="m9 15 3-3" /><path d="M17 13a6 6 0 0 0-6-6" /><path d="M21 13A10 10 0 0 0 11 3" />',
	ambulance: '<path d="M10 10H6" /><path d="M14 18V6a2 2 0 0 0-2-2H4a2 2 0 0 0-2 2v11a1 1 0 0 0 1 1h2" /><path d="M19 18h2a1 1 0 0 0 1-1v-3.28a1 1 0 0 0-.684-.948l-1.923-.641a1 1 0 0 1-.578-.502l-1.539-3.076A1 1 0 0 0 16.382 8H14" /><path d="M8 8v4" /><path d="M9 18h6" /><circle cx="17" cy="18" r="2" /><circle cx="7" cy="18" r="2" />',
	siren: '<path d="M7 18v-6a5 5 0 1 1 10 0v6" /><path d="M5 21a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-1a2 2 0 0 0-2-2H7a2 2 0 0 0-2 2z" /><path d="M21 12h1" /><path d="M18.5 4.5 18 5" /><path d="M2 12h1" /><path d="M12 2v1" /><path d="m4.929 4.929.707.707" /><path d="M12 12v6" />',
	hospital: '<path d="M12 7v4" /><path d="M14 21v-3a2 2 0 0 0-4 0v3" /><path d="M14 9h-4" /><path d="M18 11h2a2 2 0 0 1 2 2v6a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2v-9a2 2 0 0 1 2-2h2" /><path d="M18 21V5a2 2 0 0 0-2-2H8a2 2 0 0 0-2 2v16" />',
	truck: '<path d="M14 18V6a2 2 0 0 0-2-2H4a2 2 0 0 0-2 2v11a1 1 0 0 0 1 1h2" /><path d="M15 18H9" /><path d="M19 18h2a1 1 0 0 0 1-1v-3.65a1 1 0 0 0-.22-.624l-3.48-4.35A1 1 0 0 0 17.52 8H14" /><circle cx="17" cy="18" r="2" /><circle cx="7" cy="18" r="2" />',
	waypoints: '<path d="m10.586 5.414-5.172 5.172" /><path d="m18.586 13.414-5.172 5.172" /><path d="M6 12h12" /><circle cx="12" cy="20" r="2" /><circle cx="12" cy="4" r="2" /><circle cx="20" cy="12" r="2" /><circle cx="4" cy="12" r="2" />',
	bot: '<path d="M12 8V4H8" /><rect width="16" height="12" x="4" y="8" rx="2" /><path d="M2 14h2" /><path d="M20 14h2" /><path d="M15 13v2" /><path d="M9 13v2" />',
	anchor: '<path d="M12 6v16" /><path d="m19 13 2-1a9 9 0 0 1-18 0l2 1" /><path d="M9 11h6" /><circle cx="12" cy="4" r="2" />',
	fuel: '<path d="M14 13h2a2 2 0 0 1 2 2v2a2 2 0 0 0 4 0v-6.998a2 2 0 0 0-.59-1.42L18 5" /><path d="M14 21V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v16" /><path d="M2 21h13" /><path d="M3 9h11" />',
	droplets: '<path d="M7 16.3c2.2 0 4-1.83 4-4.05 0-1.16-.57-2.26-1.71-3.19S7.29 6.75 7 5.3c-.29 1.45-1.14 2.84-2.29 3.76S3 11.1 3 12.25c0 2.22 1.8 4.05 4 4.05z" /><path d="M12.56 6.6A10.97 10.97 0 0 0 14 3.02c.5 2.5 2 4.9 4 6.5s3 3.5 3 5.5a6.98 6.98 0 0 1-11.91 4.97" />',
	personStanding: '<circle cx="12" cy="5" r="1" /><path d="m9 20 3-6 3 6" /><path d="m6 8 6 2 6-2" /><path d="M12 10v4" />',
	alertTriangle: '<path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3" /><path d="M12 9v4" /><path d="M12 17h.01" />',
	mapPin: '<path d="M20 10c0 4.993-5.539 10.193-7.399 11.799a1 1 0 0 1-1.202 0C9.539 20.193 4 14.993 4 10a8 8 0 0 1 16 0" /><circle cx="12" cy="10" r="3" />',
	antenna: '<path d="M2 12 7 2" /><path d="m7 12 5-10" /><path d="m12 12 5-10" /><path d="m17 12 5-10" /><path d="M4.5 7h15" /><path d="M12 16v6" />',
};

/**
 * Primary table icon map (table byte = 47 = '/')
 * Key = symbol code byte value
 */
const PRIMARY: Record<number, string> = {
	33: I.shieldAlert,     // ! Police
	35: I.radio,           // # Digi
	36: I.phone,           // $ Phone
	38: I.router,          // & Gateway
	39: I.plane,           // ' Aircraft-S
	40: I.cloud,           // ( Cloudy
	41: I.satellite,       // ) Mobile Sat
	42: I.snowflake,       // * Snowmobile
	43: I.cross,           // + Red Cross
	44: I.compass,         // , Boy Scout
	45: I.home,            // - House QTH
	46: I.x,               // . X
	47: I.circleDot,       // / Dot
	48: I.hash,            // 0 Numeral
	58: I.flame,           // : Fire
	59: I.tent,            // ; Campground
	60: I.bike,            // < Motorcycle (closest)
	61: I.train,           // = Railroad
	62: I.car,             // > Car
	63: I.server,          // ? Server
	65: I.heartPulse,      // A Aid Station
	66: I.messageSquare,   // B BBS
	67: I.ship,            // C Canoe (closest)
	69: I.eye,             // E Eyeball
	72: I.hotel,           // H Hotel
	73: I.globe,           // I TCP/IP
	75: I.graduationCap,   // K School
	76: I.laptop,          // L Laptop
	79: I.circleDot,       // O Balloon (closest)
	80: I.shieldAlert,     // P Police
	82: I.truck,           // R RV (closest)
	83: I.rocket,          // S Shuttle
	84: I.tv,              // T SSTV
	85: I.bus,             // U Bus
	86: I.tv,              // V ATV
	87: I.cloudSun,        // W NWS Site
	88: I.plane,           // X Helicopter (closest)
	89: I.sailboat,        // Y Yacht
	91: I.personStanding,  // [ Jogger
	92: I.triangle,        // \ Triangle
	94: I.plane,           // ^ Aircraft-L
	95: I.thermometer,     // _ WX Station
	96: I.satelliteDish,   // ` Dish Ant.
	97: I.ambulance,       // a Ambulance
	98: I.bike,            // b Bike
	101: I.siren,          // e Fire Truck
	102: I.siren,          // f Fire Truck
	104: I.hospital,       // h Hospital
	105: I.radio,          // i IOTA
	106: I.car,            // j Jeep
	107: I.truck,          // k Truck
	110: I.waypoints,      // n Node
	112: I.bot,            // p Rover
	114: I.antenna,        // r Antenna
	115: I.anchor,         // s Ship
	116: I.fuel,           // t Truck Stop
	117: I.truck,          // u Truck-18
	118: I.car,            // v Van
	119: I.droplets,       // w Water
	121: I.home,           // y House-Yagi
};

/**
 * Alternate table icon map (table byte = 92 = '\')
 */
const ALTERNATE: Record<number, string> = {
	33: I.alertTriangle,   // ! Emergency
	35: I.radio,           // # Overlay Digi
	45: I.home,            // - House-HF
	62: I.car,             // > Car
	72: I.cloud,           // H Haze
	79: I.shieldAlert,     // O EOC
	87: I.cloudSun,        // W NWS Site
	95: I.thermometer,     // _ WX Station
	110: I.waypoints,      // n Node
	115: I.anchor,         // s Ship
};

/**
 * Get the inner SVG elements for an APRS symbol.
 */
function getIconInner(sym: APRSSymbol): string {
	if (!sym) return I.mapPin;
	const table = sym.table === 47 ? PRIMARY : ALTERNATE;
	return table[sym.code] || I.mapPin;
}

/**
 * Wrap icon inner elements in an SVG tag with given size and stroke.
 */
function wrapSvg(inner: string, size: number, stroke: string, strokeWidth: number): string {
	return `<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="${stroke}" stroke-width="${strokeWidth}" stroke-linecap="round" stroke-linejoin="round">${inner}</svg>`;
}

/**
 * Generate HTML for a Leaflet divIcon marker.
 * Colored circle background with white lucide icon.
 */
export function createMarkerHtml(sym: APRSSymbol, color: string, selected: boolean): string {
	const inner = getIconInner(sym);
	const size = selected ? 36 : 28;
	const iconSize = selected ? 20 : 14;
	const strokeWidth = selected ? 2.2 : 2.4;
	const svg = wrapSvg(inner, iconSize, '#fff', strokeWidth);
	const shadow = selected
		? '0 2px 8px rgba(0,0,0,0.5), 0 0 0 3px rgba(255,255,255,0.8)'
		: '0 1px 4px rgba(0,0,0,0.4)';
	const border = selected ? '2px solid #fff' : '2px solid rgba(255,255,255,0.3)';

	return `<div style="width:${size}px;height:${size}px;border-radius:50%;background:${color};border:${border};display:flex;align-items:center;justify-content:center;box-shadow:${shadow};transition:all 0.15s ease;">${svg}</div>`;
}

/**
 * Create Leaflet divIcon options for a station marker.
 */
export function createStationIcon(sym: APRSSymbol, color: string, selected: boolean): { className: string; html: string; iconSize: [number, number]; iconAnchor: [number, number] } {
	const size = selected ? 36 : 28;
	return {
		className: 'aprs-station-icon',
		html: createMarkerHtml(sym, color, selected),
		iconSize: [size, size],
		iconAnchor: [size / 2, size / 2],
	};
}
