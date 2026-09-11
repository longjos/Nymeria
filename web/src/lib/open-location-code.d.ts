/**
 * Ambient module declaration for 'open-location-code' (v1.0.3).
 *
 * The package ships no types, and the DefinitelyTyped package
 * (@types/open-location-code) declares every method `static`, which does not
 * match this package's actual runtime shape — every method (isValid, decode,
 * encode, isShort, isFull, recoverNearest, shorten) is defined on
 * `OpenLocationCode.prototype`, i.e. called on an *instance*
 * (`new OpenLocationCode()`), never on the class itself. Verified directly
 * against node_modules/open-location-code/openlocationcode.js. Installing
 * @types/open-location-code alongside this file would conflict — don't.
 */
declare module 'open-location-code' {
	export interface CodeArea {
		latitudeLo: number;
		longitudeLo: number;
		latitudeHi: number;
		longitudeHi: number;
		codeLength: number;
		latitudeCenter: number;
		longitudeCenter: number;
	}

	export class OpenLocationCode {
		isValid(code: string): boolean;
		isShort(code: string): boolean;
		isFull(code: string): boolean;
		encode(latitude: number, longitude: number, codeLength?: number): string;
		decode(code: string): CodeArea;
		recoverNearest(shortCode: string, latitude: number, longitude: number): string;
		shorten(code: string, latitude: number, longitude: number): string;
	}
}
