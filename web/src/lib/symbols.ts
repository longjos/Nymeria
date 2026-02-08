import type { APRSSymbol } from './types';

// Map APRS symbol codes to display info: [label, color, cssClass]
// Primary table (table = 47 = '/')
const PRIMARY: Record<number, [string, string]> = {
	33: ['Police', '#e74c3c'],         // !
	35: ['Digi', '#9b59b6'],           // #
	36: ['Phone', '#3498db'],          // $
	38: ['Gateway', '#2ecc71'],        // &
	39: ['Aircraft-S', '#e67e22'],     // '
	40: ['Cloudy', '#95a5a6'],         // (
	41: ['Mobile Sat', '#1abc9c'],     // )
	42: ['Snowmobile', '#ecf0f1'],     // *
	43: ['Red Cross', '#e74c3c'],      // +
	44: ['Boy Scout', '#27ae60'],      // ,
	45: ['House QTH', '#3498db'],      // -
	46: ['X', '#e74c3c'],             // .
	47: ['Dot', '#e74c3c'],           // /
	48: ['Numeral', '#7f8c8d'],        // 0
	58: ['Fire', '#e74c3c'],           // :
	59: ['Campground', '#27ae60'],     // ;
	60: ['Motorcycle', '#e67e22'],     // <
	61: ['Railroad', '#8e44ad'],       // =
	62: ['Car', '#3498db'],            // >
	63: ['Server', '#2c3e50'],         // ?
	65: ['Aid Station', '#e74c3c'],    // A
	66: ['BBS', '#8e44ad'],            // B
	67: ['Canoe', '#1abc9c'],          // C
	69: ['Eyeball', '#f39c12'],        // E
	72: ['Hotel', '#9b59b6'],          // H
	73: ['TCP/IP', '#2ecc71'],         // I
	75: ['School', '#f1c40f'],         // K
	76: ['Laptop', '#3498db'],         // L
	79: ['Balloon', '#e74c3c'],        // O
	80: ['Police', '#e74c3c'],         // P
	82: ['RV', '#27ae60'],             // R
	83: ['Shuttle', '#e67e22'],        // S
	84: ['SSTV', '#8e44ad'],           // T
	85: ['Bus', '#f39c12'],            // U
	86: ['ATV', '#1abc9c'],            // V
	87: ['NWS Site', '#3498db'],       // W
	88: ['Helicopter', '#e74c3c'],     // X
	89: ['Yacht', '#2980b9'],          // Y
	91: ['Jogger', '#27ae60'],         // [
	92: ['Triangle', '#e67e22'],       // \
	94: ['Aircraft-L', '#e67e22'],     // ^
	95: ['WX Station', '#3498db'],     // _
	96: ['Dish Ant.', '#7f8c8d'],      // `
	97: ['Ambulance', '#e74c3c'],      // a
	98: ['Bike', '#27ae60'],           // b
	101: ['Fire Truck', '#e74c3c'],    // e
	102: ['Fire Truck', '#e74c3c'],    // f (corrected later)
	104: ['Hospital', '#e74c3c'],      // h
	105: ['IOTA', '#3498db'],          // i
	106: ['Jeep', '#8b4513'],          // j
	107: ['Truck', '#e67e22'],         // k
	110: ['Node', '#2ecc71'],          // n
	112: ['Rover', '#9b59b6'],         // p
	114: ['Antenna', '#7f8c8d'],       // r
	115: ['Ship', '#2980b9'],          // s
	116: ['Truck Stop', '#e67e22'],    // t
	117: ['Truck-18', '#e67e22'],      // u
	118: ['Van', '#3498db'],           // v
	119: ['Water', '#2980b9'],         // w
	121: ['House-Yagi', '#3498db'],    // y
};

// Alternate table (table = 92 = '\')
const ALTERNATE: Record<number, [string, string]> = {
	33: ['Emergency', '#e74c3c'],      // !
	35: ['Overlay Digi', '#9b59b6'],   // #
	45: ['House-HF', '#3498db'],       // -
	62: ['Car', '#e74c3c'],            // >
	72: ['Haze', '#95a5a6'],           // H
	79: ['EOC', '#e74c3c'],            // O
	87: ['NWS Site', '#3498db'],       // W
	95: ['WX Station', '#3498db'],     // _
	110: ['Node', '#2ecc71'],          // n
	115: ['Ship', '#2980b9'],          // s
};

export function symbolInfo(sym: APRSSymbol): { label: string; color: string } {
	if (!sym) return { label: 'Unknown', color: '#7f8c8d' };

	const table = sym.table === 47 ? PRIMARY : ALTERNATE; // 47 = '/'
	const entry = table[sym.code];
	if (entry) return { label: entry[0], color: entry[1] };

	return { label: String.fromCharCode(sym.code), color: '#7f8c8d' };
}

export function symbolChar(sym: APRSSymbol): string {
	if (!sym) return '?';
	return String.fromCharCode(sym.code);
}

export function markerColor(sym: APRSSymbol): string {
	return symbolInfo(sym).color;
}
