// NWS hard-wrap normaliser. Mobile review P1-1 (scratchpad/mobile-review.md):
// NWS ships instruction/description text hard-wrapped at ~68 columns — a
// single "\n" inside a paragraph is only a wrap artifact, "\n\n" is a real
// paragraph break. `nwsBlocks()` joins ONLY the wrap artifacts while keeping
// the line breaks NWS uses on purpose: "* WHAT..." style bullet items,
// "- " sub-bullets, ALLCAPS "LABEL..." lines (HAZARD/SOURCE/IMPACT,
// LAT...LON, TIME...MOT...LOC), "&&"/"$$" terminators, and aligned
// river-gauge tables (kept verbatim, line breaks intact).
//
// Algorithm: split on blank-line(s) into paragraphs. Within a paragraph, if
// any line looks like an aligned table column (a run of 3+ spaces between
// non-space runs — wide enough that it can't be an ordinary double-space-
// after-period sentence), keep the whole paragraph as one preformatted
// block. Otherwise walk the paragraph's lines: a line that starts a new
// structural unit (bullet, sub-bullet, ALLCAPS label, terminator) flushes
// whatever was being built and starts a new item; every other line is a
// wrap continuation and gets appended (joined with a single space) to
// whatever item/paragraph is currently open.

export type WxBlock =
	| { kind: 'p'; text: string }
	| { kind: 'item'; text: string; label?: string; sub?: boolean }
	| { kind: 'pre'; text: string };

// Aligned columns: two non-space runs separated by 3+ spaces. Prose —
// including NWS's own double-space-after-a-period convention — never has a
// run that wide; gauge tables always do (columns are padded to align).
const TABLE_LINE = /\S {3,}\S/;

// "* WHAT...text" / "* WHERE...text" / "* WHEN...text" / "* IMPACTS...text".
const BULLET_RE = /^\* ([A-Z][A-Z0-9 ]*)\.\.\.\s*(.*)$/;

// "- sub-bullet text" (county lists etc. under a WHERE bullet).
const SUBBULLET_RE = /^-\s+(.*)$/;

// Generic ALLCAPS "LABEL..." lines: HAZARD..., SOURCE..., IMPACT...,
// LAT...LON ..., TIME...MOT...LOC ... . Deliberately does not try to split
// out the label for these (LAT...LON and TIME...MOT...LOC have more than
// one "..." run) — the whole line is kept verbatim and just isolated from
// its neighbours so it is never joined into surrounding prose.
const LABEL_LINE = /^[A-Z][A-Z0-9 ]*\.\.\./;

const TERMINATORS = new Set(['&&', '$$']);

interface Building {
	kind: 'p' | 'item';
	label?: string;
	sub?: boolean;
	parts: string[];
}

export function nwsBlocks(raw: string): WxBlock[] {
	if (!raw) return [];

	// Normalise line endings, then trim trailing whitespace off every line —
	// a whitespace-only line is a paragraph break same as a truly empty one,
	// and stray trailing spaces on a wrapped line must never survive into the
	// joined text.
	const lines = raw
		.replace(/\r\n?/g, '\n')
		.split('\n')
		.map((l) => l.replace(/[ \t]+$/, ''));

	const paragraphs: string[][] = [];
	let cur: string[] = [];
	for (const line of lines) {
		if (line === '') {
			if (cur.length) paragraphs.push(cur);
			cur = [];
		} else {
			cur.push(line);
		}
	}
	if (cur.length) paragraphs.push(cur);

	const blocks: WxBlock[] = [];
	for (const paragraph of paragraphs) blocks.push(...blockifyParagraph(paragraph));
	return blocks;
}

function blockifyParagraph(paragraphLines: string[]): WxBlock[] {
	if (paragraphLines.some((l) => TABLE_LINE.test(l))) {
		return [{ kind: 'pre', text: paragraphLines.join('\n') }];
	}

	const blocks: WxBlock[] = [];
	let building: Building | null = null;

	const flush = () => {
		if (!building) return;
		const text = building.parts.join(' ').trim();
		blocks.push(
			building.kind === 'item'
				? { kind: 'item', text, label: building.label, sub: building.sub }
				: { kind: 'p', text }
		);
		building = null;
	};

	for (const rawLine of paragraphLines) {
		const line = rawLine.trim();
		const bullet = BULLET_RE.exec(line);
		const sub = SUBBULLET_RE.exec(line);

		if (bullet) {
			flush();
			building = { kind: 'item', label: bullet[1], parts: [bullet[2]] };
		} else if (sub) {
			flush();
			building = { kind: 'item', sub: true, parts: [sub[1]] };
		} else if (TERMINATORS.has(line)) {
			flush();
			blocks.push({ kind: 'item', text: line });
		} else if (LABEL_LINE.test(line)) {
			flush();
			building = { kind: 'item', parts: [line] };
		} else if (building) {
			building.parts.push(line);
		} else {
			building = { kind: 'p', parts: [line] };
		}
	}
	flush();
	return blocks;
}

// Field summary for the collapsed Description preview: the WHAT/WHERE/WHEN/
// IMPACTS bullets ARE the summary NWS writers intend readers to scan first.
// Falls back to the first two blocks when the product has no labelled
// bullets (e.g. a plain narrative Special Weather Statement).
const PREVIEW_LABELS = new Set(['WHAT', 'WHERE', 'WHEN', 'IMPACTS']);

export function previewBlocks(blocks: WxBlock[]): WxBlock[] {
	const labelled = blocks.filter((b): b is WxBlock & { kind: 'item'; label: string } => b.kind === 'item' && !!b.label && PREVIEW_LABELS.has(b.label));
	if (labelled.length) return labelled;
	return blocks.slice(0, 2);
}
