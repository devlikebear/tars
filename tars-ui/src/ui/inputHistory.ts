export type HistoryDirection = 'up' | 'down';

export type InputHistoryState = {
	entries: string[];
	index: number;
	draft: string;
};

export const initialInputHistoryState: InputHistoryState = {
	entries: [],
	index: -1,
	draft: '',
};

export type NavigateResult = {
	state: InputHistoryState;
	value: string;
	changed: boolean;
};

export function pushInputHistory(state: InputHistoryState, value: string): InputHistoryState {
	const trimmed = value.trim();
	if (trimmed === '') {
		return {
			...state,
			index: -1,
			draft: '',
		};
	}
	const entries = [...state.entries];
	const last = entries[entries.length - 1] ?? '';
	if (last !== value) {
		entries.push(value);
	}
	return {
		entries,
		index: -1,
		draft: '',
	};
}

export function navigateInputHistory(
	state: InputHistoryState,
	currentValue: string,
	direction: HistoryDirection,
): NavigateResult {
	if (state.entries.length === 0) {
		return {state, value: currentValue, changed: false};
	}
	if (direction === 'up') {
		if (state.index === -1) {
			const nextIndex = state.entries.length - 1;
			return {
				state: {...state, index: nextIndex, draft: currentValue},
				value: state.entries[nextIndex] ?? currentValue,
				changed: true,
			};
		}
		const nextIndex = Math.max(0, state.index - 1);
		if (nextIndex === state.index) {
			return {state, value: currentValue, changed: false};
		}
		return {
			state: {...state, index: nextIndex},
			value: state.entries[nextIndex] ?? currentValue,
			changed: true,
		};
	}

	if (state.index === -1) {
		return {state, value: currentValue, changed: false};
	}
	const nextIndex = state.index + 1;
	if (nextIndex < state.entries.length) {
		return {
			state: {...state, index: nextIndex},
			value: state.entries[nextIndex] ?? currentValue,
			changed: true,
		};
	}
	return {
		state: {...state, index: -1},
		value: state.draft,
		changed: true,
	};
}

export function resetInputHistoryCursor(state: InputHistoryState): InputHistoryState {
	if (state.index === -1 && state.draft === '') {
		return state;
	}
	return {
		...state,
		index: -1,
		draft: '',
	};
}
