/**
 * Neutral alias for the app's unit system.
 *
 * The miles-vs-kilometres preference happens to be stored in the weather config,
 * but a road-distance readout must not have to import a store named "weather" to
 * decide what to show. Import `unitSystem` from here; it is the same store, so
 * the app can never contradict itself.
 */
export { weatherUnits as unitSystem } from './weather';
