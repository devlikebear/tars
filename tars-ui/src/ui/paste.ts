export const bracketedPasteStart = '\x1b[200~';
export const bracketedPasteEnd = '\x1b[201~';

export type PasteState = {
	inPaste: boolean;
	buffer: string;
};

export type PasteChunk = {
	type: 'text' | 'paste';
	value: string;
};

export type PasteConsumeResult = {
	state: PasteState;
	chunks: PasteChunk[];
};

export function consumeBracketedPaste(state: PasteState, input: string): PasteConsumeResult {
	let inPaste = state.inPaste;
	let buffer = state.buffer;
	let remaining = input;
	const chunks: PasteChunk[] = [];

	if (inPaste) {
		const merged = buffer + remaining;
		const endIdx = merged.indexOf(bracketedPasteEnd);
		if (endIdx === -1) {
			return {
				state: {inPaste: true, buffer: merged},
				chunks: [],
			};
		}
		chunks.push({type: 'paste', value: merged.slice(0, endIdx)});
		remaining = merged.slice(endIdx + bracketedPasteEnd.length);
		inPaste = false;
		buffer = '';
	}

	for (;;) {
		const startIdx = remaining.indexOf(bracketedPasteStart);
		if (startIdx === -1) {
			if (remaining !== '') {
				chunks.push({type: 'text', value: remaining});
			}
			break;
		}
		if (startIdx > 0) {
			chunks.push({type: 'text', value: remaining.slice(0, startIdx)});
		}
		remaining = remaining.slice(startIdx + bracketedPasteStart.length);

		const endIdx = remaining.indexOf(bracketedPasteEnd);
		if (endIdx === -1) {
			inPaste = true;
			buffer = remaining;
			break;
		}
		chunks.push({type: 'paste', value: remaining.slice(0, endIdx)});
		remaining = remaining.slice(endIdx + bracketedPasteEnd.length);
	}

	return {
		state: {inPaste, buffer},
		chunks,
	};
}
