export type InputEditState = {
	value: string;
	cursor: number;
};

export type DeleteResult = {
	next: InputEditState;
	deleted: string;
	start: number;
	end: number;
};

function clampCursor(value: string, cursor: number): number {
	return Math.max(0, Math.min(cursor, value.length));
}

export function normalizeState(state: InputEditState): InputEditState {
	return {value: state.value, cursor: clampCursor(state.value, state.cursor)};
}

export function insertText(state: InputEditState, text: string): InputEditState {
	if (text === '') {
		return normalizeState(state);
	}
	const current = normalizeState(state);
	const nextValue = current.value.slice(0, current.cursor) + text + current.value.slice(current.cursor);
	return {value: nextValue, cursor: current.cursor + text.length};
}

export function backspace(state: InputEditState): InputEditState {
	const current = normalizeState(state);
	if (current.cursor <= 0) {
		return current;
	}
	return {
		value: current.value.slice(0, current.cursor - 1) + current.value.slice(current.cursor),
		cursor: current.cursor - 1,
	};
}

export function replaceRange(state: InputEditState, start: number, end: number, replacement: string): InputEditState {
	const current = normalizeState(state);
	const clampedStart = Math.max(0, Math.min(start, current.value.length));
	const clampedEnd = Math.max(clampedStart, Math.min(end, current.value.length));
	const nextValue = current.value.slice(0, clampedStart) + replacement + current.value.slice(clampedEnd);
	return {value: nextValue, cursor: clampedStart + replacement.length};
}

function lineStartIndex(value: string, cursor: number): number {
	const clamped = clampCursor(value, cursor);
	const prevNewline = value.lastIndexOf('\n', clamped - 1);
	return prevNewline === -1 ? 0 : prevNewline + 1;
}

function lineEndIndex(value: string, cursor: number): number {
	const clamped = clampCursor(value, cursor);
	const nextNewline = value.indexOf('\n', clamped);
	return nextNewline === -1 ? value.length : nextNewline;
}

export function deleteToLineStart(state: InputEditState): DeleteResult {
	const current = normalizeState(state);
	const start = lineStartIndex(current.value, current.cursor);
	const end = current.cursor;
	return {
		next: replaceRange(current, start, end, ''),
		deleted: current.value.slice(start, end),
		start,
		end,
	};
}

export function deleteToLineEnd(state: InputEditState): DeleteResult {
	const current = normalizeState(state);
	const start = current.cursor;
	const end = lineEndIndex(current.value, current.cursor);
	return {
		next: replaceRange(current, start, end, ''),
		deleted: current.value.slice(start, end),
		start,
		end,
	};
}
