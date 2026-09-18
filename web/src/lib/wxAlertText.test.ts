// Table-driven tests for the NWS hard-wrap normaliser. Mobile review P1-1
// (scratchpad/mobile-review.md): NWS ships instruction/description text
// hard-wrapped at ~68 columns — a single "\n" inside a paragraph is just a
// wrap artifact, "\n\n" is a real paragraph break, and several line shapes
// (bullets, sub-bullets, ALLCAPS label lines, terminators, gauge tables) are
// structural and must never be joined into the prose flow around them.
import { describe, it, expect } from 'vitest';
import { nwsBlocks, previewBlocks, type WxBlock } from './wxAlertText';

describe('nwsBlocks — basic shapes', () => {
	it('returns an empty array for an empty string', () => {
		expect(nwsBlocks('')).toEqual([]);
	});

	it('returns an empty array for whitespace-only input', () => {
		expect(nwsBlocks('   \n  \n\t\n')).toEqual([]);
	});

	it('renders a single line as a single paragraph block', () => {
		expect(nwsBlocks('Just one line.')).toEqual([{ kind: 'p', text: 'Just one line.' }]);
	});

	it('joins a hard-wrapped two-line paragraph with a single space', () => {
		const raw = 'Turn around, dont drown when encountering flooded roads. Most flood\ndeaths occur in vehicles.';
		expect(nwsBlocks(raw)).toEqual([
			{ kind: 'p', text: 'Turn around, dont drown when encountering flooded roads. Most flood deaths occur in vehicles.' }
		]);
	});

	it('keeps two blank-line-separated paragraphs distinct (does not join across paragraphs)', () => {
		const raw = 'Stay away or be swept away. River banks and culverts can become\nunstable and unsafe.\n\nBe especially cautious at night when it is harder to recognize the\ndangers of flooding.';
		expect(nwsBlocks(raw)).toEqual([
			{ kind: 'p', text: 'Stay away or be swept away. River banks and culverts can become unstable and unsafe.' },
			{ kind: 'p', text: 'Be especially cautious at night when it is harder to recognize the dangers of flooding.' }
		]);
	});

	it('treats runs of 2+ blank lines the same as a single blank line (no empty blocks)', () => {
		const raw = 'Paragraph one.\n\n\n\nParagraph two.';
		expect(nwsBlocks(raw)).toEqual([
			{ kind: 'p', text: 'Paragraph one.' },
			{ kind: 'p', text: 'Paragraph two.' }
		]);
	});

	it('normalises CRLF line endings identically to LF', () => {
		const crlf = 'Line one\r\nLine two\r\n\r\nLine three';
		const lf = 'Line one\nLine two\n\nLine three';
		expect(nwsBlocks(crlf)).toEqual(nwsBlocks(lf));
		expect(nwsBlocks(crlf)).toEqual([
			{ kind: 'p', text: 'Line one Line two' },
			{ kind: 'p', text: 'Line three' }
		]);
	});

	it('normalises a lone CR (old Mac line ending) the same as LF', () => {
		expect(nwsBlocks('Line one\rLine two')).toEqual([{ kind: 'p', text: 'Line one Line two' }]);
	});

	it('strips trailing whitespace on wrapped lines without introducing a double space when joined', () => {
		const raw = 'Line one has trailing spaces   \nLine two continues normally.';
		expect(nwsBlocks(raw)).toEqual([{ kind: 'p', text: 'Line one has trailing spaces Line two continues normally.' }]);
	});

	it('handles trailing whitespace at the very end of the input', () => {
		expect(nwsBlocks('Single line with trailing space.   \n\n')).toEqual([{ kind: 'p', text: 'Single line with trailing space.' }]);
	});

	it('passes through a single-line paragraph that is exactly 68 characters unchanged', () => {
		const line = 'A'.repeat(67) + '.'; // exactly 68 chars
		expect(line.length).toBe(68);
		expect(nwsBlocks(line)).toEqual([{ kind: 'p', text: line }]);
	});

	it('handles text with no wrapping at all — every sentence its own blank-line-delimited paragraph', () => {
		const raw = 'Sentence one stands alone.\n\nSentence two stands alone.\n\nSentence three stands alone.';
		expect(nwsBlocks(raw)).toEqual([
			{ kind: 'p', text: 'Sentence one stands alone.' },
			{ kind: 'p', text: 'Sentence two stands alone.' },
			{ kind: 'p', text: 'Sentence three stands alone.' }
		]);
	});
});

describe('nwsBlocks — "* LABEL..." bullet items', () => {
	it('splits a bullet line into a labelled item and joins its wrapped continuation', () => {
		const raw = '* WHAT...Flooding caused by excessive rainfall continues.';
		expect(nwsBlocks(raw)).toEqual([{ kind: 'item', label: 'WHAT', text: 'Flooding caused by excessive rainfall continues.' }]);
	});

	it('joins a wrapped bullet continuation line (no blank line before it) into the same item', () => {
		const raw = '* WHERE...Portions of central Indiana, including the following\ncounties, Adams, Allen, Wells and Huntington.';
		expect(nwsBlocks(raw)).toEqual([
			{ kind: 'item', label: 'WHERE', text: 'Portions of central Indiana, including the following counties, Adams, Allen, Wells and Huntington.' }
		]);
	});

	it('keeps consecutive WHAT/WHERE/WHEN/IMPACTS bullets as separate items, blank-line separated', () => {
		const raw = [
			'* WHAT...Flooding caused by excessive rainfall continues.',
			'',
			'* WHERE...Portions of central Indiana.',
			'',
			'* WHEN...Until further notice.',
			'',
			'* IMPACTS...Flooding of rivers, creeks, streams, and other low-lying\nareas as well as normally dry areas.'
		].join('\n');
		expect(nwsBlocks(raw)).toEqual([
			{ kind: 'item', label: 'WHAT', text: 'Flooding caused by excessive rainfall continues.' },
			{ kind: 'item', label: 'WHERE', text: 'Portions of central Indiana.' },
			{ kind: 'item', label: 'WHEN', text: 'Until further notice.' },
			{ kind: 'item', label: 'IMPACTS', text: 'Flooding of rivers, creeks, streams, and other low-lying areas as well as normally dry areas.' }
		]);
	});

	it('flushes and starts a new item at a bullet even without a blank line before it', () => {
		// Real NWS text sometimes omits the blank line between bullets.
		const raw = '* WHAT...Severe thunderstorm warning.\n* WHERE...Grant County.';
		expect(nwsBlocks(raw)).toEqual([
			{ kind: 'item', label: 'WHAT', text: 'Severe thunderstorm warning.' },
			{ kind: 'item', label: 'WHERE', text: 'Grant County.' }
		]);
	});
});

describe('nwsBlocks — "- " sub-bullets', () => {
	it('renders sub-bullets under a bullet as separate sub items, not merged with the bullet or each other', () => {
		const raw = '* WHERE...Portions of Grant County, including the following areas\n- Marion\n- Fairmount';
		expect(nwsBlocks(raw)).toEqual([
			{ kind: 'item', label: 'WHERE', text: 'Portions of Grant County, including the following areas' },
			{ kind: 'item', text: 'Marion', sub: true },
			{ kind: 'item', text: 'Fairmount', sub: true }
		]);
	});

	it('joins a wrapped sub-bullet continuation into the same sub item', () => {
		const raw = '- A long county name that would need to wrap across\n  two lines in the original product';
		expect(nwsBlocks(raw)).toEqual([
			{ kind: 'item', text: 'A long county name that would need to wrap across two lines in the original product', sub: true }
		]);
	});
});

describe('nwsBlocks — ALLCAPS label lines (HAZARD/SOURCE/IMPACT)', () => {
	it('keeps HAZARD/SOURCE/IMPACT lines distinct even with no blank lines between them', () => {
		const raw = 'HAZARD...60 mph wind gusts and quarter size hail.\nSOURCE...Radar indicated.\nIMPACT...Expect damage to roofs, siding, and trees.';
		expect(nwsBlocks(raw)).toEqual([
			{ kind: 'item', text: 'HAZARD...60 mph wind gusts and quarter size hail.' },
			{ kind: 'item', text: 'SOURCE...Radar indicated.' },
			{ kind: 'item', text: 'IMPACT...Expect damage to roofs, siding, and trees.' }
		]);
	});

	it('joins a wrapped continuation onto a label line', () => {
		const raw = 'HAZARD...Flooding caused by heavy rain across the region continues\nthrough the overnight hours into tomorrow morning.';
		expect(nwsBlocks(raw)).toEqual([
			{ kind: 'item', text: 'HAZARD...Flooding caused by heavy rain across the region continues through the overnight hours into tomorrow morning.' }
		]);
	});

	it('preserves a LAT...LON coordinate line as its own item, not merged with neighbours', () => {
		const raw = 'Severe thunderstorm warning for...\nLAT...LON 4023 8583 4023 8547 4011 8547 4011 8583\n&&';
		expect(nwsBlocks(raw)).toEqual([
			{ kind: 'p', text: 'Severe thunderstorm warning for...' },
			{ kind: 'item', text: 'LAT...LON 4023 8583 4023 8547 4011 8547 4011 8583' },
			{ kind: 'item', text: '&&' }
		]);
	});

	it('preserves a TIME...MOT...LOC line as its own item', () => {
		const raw = 'TIME...MOT...LOC 2215Z 254DEG 39KT 3944 8595 3937 8608';
		expect(nwsBlocks(raw)).toEqual([{ kind: 'item', text: 'TIME...MOT...LOC 2215Z 254DEG 39KT 3944 8595 3937 8608' }]);
	});
});

describe('nwsBlocks — && and $$ terminators', () => {
	it('keeps && on its own line even directly after a wrapped bullet with no blank line', () => {
		const raw = '* IMPACTS...Flooding of rivers, creeks, streams, and other low-lying\nareas as well as normally dry areas.\n&&';
		expect(nwsBlocks(raw)).toEqual([
			{ kind: 'item', label: 'IMPACTS', text: 'Flooding of rivers, creeks, streams, and other low-lying areas as well as normally dry areas.' },
			{ kind: 'item', text: '&&' }
		]);
	});

	it('keeps $$ as its own block', () => {
		expect(nwsBlocks('Some closing remark line.\n$$')).toEqual([
			{ kind: 'p', text: 'Some closing remark line.' },
			{ kind: 'item', text: '$$' }
		]);
	});

	it('does not merge text that follows a terminator into it', () => {
		const raw = '&&\nA fresh unrelated paragraph line.';
		expect(nwsBlocks(raw)).toEqual([
			{ kind: 'item', text: '&&' },
			{ kind: 'p', text: 'A fresh unrelated paragraph line.' }
		]);
	});
});

describe('nwsBlocks — aligned river-gauge tables', () => {
	it('keeps an aligned-column table as a single preformatted block with original line breaks intact', () => {
		const raw = ['Location            Fld Stg  Observed  Forecast', 'Huntington              10.0      9.5 Mon      9.8 Tue', 'Wabash River            12.0     11.2 Fri     11.8 Fri'].join('\n');
		expect(nwsBlocks(raw)).toEqual([{ kind: 'pre', text: raw }]);
	});

	it('does not misdetect an ordinary double-space-after-period sentence as a table', () => {
		// Old-style double space after a period is common in NWS prose and must
		// NOT trip the 3+-space table heuristic (regression guard for that
		// threshold — 2 spaces must not be enough).
		const raw = "Turn around, don't drown.  Most flood deaths occur in vehicles.";
		expect(nwsBlocks(raw)).toEqual([{ kind: 'p', text: "Turn around, don't drown.  Most flood deaths occur in vehicles." }]);
	});

	it('detects a table via just one aligned-column line among otherwise plain lines in the same paragraph', () => {
		const raw = 'River gauge readings follow.\nHuntington              10.0      9.5 Mon      9.8 Tue';
		expect(nwsBlocks(raw)).toEqual([{ kind: 'pre', text: 'River gauge readings follow.\nHuntington              10.0      9.5 Mon      9.8 Tue' }]);
	});
});

describe('nwsBlocks — content preservation (never drop text)', () => {
	it('preserves every word across a mixed paragraph of bullets, sub-bullets and wraps', () => {
		const raw = [
			'* WHAT...Areal flood advisory for excessive runoff from heavy rain.',
			'',
			'* WHERE...Portions of central Indiana, including the following\ncounties',
			'- Adams',
			'- Allen',
			'- Wells',
			'',
			'* WHEN...Until 800 PM EDT.',
			'',
			'* IMPACTS...Minor flooding in low-lying and poor drainage areas.',
			'',
			'HAZARD...Excessive rainfall.',
			'SOURCE...Doppler radar and automated rain gauges.',
			'&&'
		].join('\n');
		const blocks = nwsBlocks(raw);
		const rebuilt = blocks.map((b) => (b.kind === 'item' && b.label ? `${b.label} ${b.text}` : b.text)).join(' ');
		for (const word of ['WHAT', 'excessive', 'WHERE', 'Adams', 'Allen', 'Wells', 'WHEN', '800', 'IMPACTS', 'Minor', 'HAZARD', 'SOURCE', 'Doppler', '&&']) {
			expect(rebuilt).toContain(word);
		}
		// And every non-blank source line contributed to exactly the right shape.
		expect(blocks).toEqual([
			{ kind: 'item', label: 'WHAT', text: 'Areal flood advisory for excessive runoff from heavy rain.' },
			{ kind: 'item', label: 'WHERE', text: 'Portions of central Indiana, including the following counties' },
			{ kind: 'item', text: 'Adams', sub: true },
			{ kind: 'item', text: 'Allen', sub: true },
			{ kind: 'item', text: 'Wells', sub: true },
			{ kind: 'item', label: 'WHEN', text: 'Until 800 PM EDT.' },
			{ kind: 'item', label: 'IMPACTS', text: 'Minor flooding in low-lying and poor drainage areas.' },
			{ kind: 'item', text: 'HAZARD...Excessive rainfall.' },
			{ kind: 'item', text: 'SOURCE...Doppler radar and automated rain gauges.' },
			{ kind: 'item', text: '&&' }
		]);
	});

	it('never produces an item with empty label text for a bullet with no body (label still visible)', () => {
		const raw = '* WHAT...';
		expect(nwsBlocks(raw)).toEqual([{ kind: 'item', label: 'WHAT', text: '' }]);
	});
});

describe('previewBlocks', () => {
	const bulletFixture = (): WxBlock[] =>
		nwsBlocks(
			[
				'* WHAT...Flooding caused by excessive rainfall continues.',
				'',
				'* WHERE...Portions of central Indiana.',
				'',
				'* WHEN...Until further notice.',
				'',
				'* IMPACTS...Flooding of rivers, creeks and streams.',
				'',
				'HAZARD...Excessive rainfall.',
				'&&'
			].join('\n')
		);

	it('selects only the WHAT/WHERE/WHEN/IMPACTS items when present, in order, skipping other blocks', () => {
		const preview = previewBlocks(bulletFixture());
		expect(preview).toEqual([
			{ kind: 'item', label: 'WHAT', text: 'Flooding caused by excessive rainfall continues.' },
			{ kind: 'item', label: 'WHERE', text: 'Portions of central Indiana.' },
			{ kind: 'item', label: 'WHEN', text: 'Until further notice.' },
			{ kind: 'item', label: 'IMPACTS', text: 'Flooding of rivers, creeks and streams.' }
		]);
	});

	it('falls back to the first two blocks when there are no labelled WHAT/WHERE/WHEN/IMPACTS items', () => {
		const blocks = nwsBlocks('Paragraph one.\n\nParagraph two.\n\nParagraph three.');
		expect(previewBlocks(blocks)).toEqual([
			{ kind: 'p', text: 'Paragraph one.' },
			{ kind: 'p', text: 'Paragraph two.' }
		]);
	});

	it('returns an empty array for an empty block list', () => {
		expect(previewBlocks([])).toEqual([]);
	});

	it('falls back to whatever is available when fewer than two blocks exist', () => {
		expect(previewBlocks([{ kind: 'p', text: 'Only one.' }])).toEqual([{ kind: 'p', text: 'Only one.' }]);
	});
});
