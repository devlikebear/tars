import React, {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {Text, useInput, useStdin} from 'ink';
import chalk from 'chalk';
import {consumeBracketedPaste, PasteState} from './paste.js';
import {KillRing} from './killRing.js';
import {backspace, deleteToLineEnd, deleteToLineStart, insertText, replaceRange} from './inputEdit.js';
import {UndoStack} from './undoStack.js';
import {completeCommandInput} from '../commands/complete.js';
import {
	initialInputHistoryState,
	navigateInputHistory,
	pushInputHistory,
	resetInputHistoryCursor,
} from './inputHistory.js';

type CustomTextInputProps = {
	value: string;
	onChange: (value: string) => void;
	onSubmit: () => void;
	onEscape?: () => void;
	placeholder?: string;
	focus?: boolean;
	showCursor?: boolean;
	enableHistoryNavigation?: boolean;
};

const initialPasteState: PasteState = {inPaste: false, buffer: ''};

type UndoSnapshot = {
	value: string;
	cursor: number;
};

type ActionType = 'none' | 'kill' | 'yank';

type Range = {
	start: number;
	end: number;
};

function writeTTY(data: string): void {
	if (!process.stdout.isTTY) {
		return;
	}
	try {
		process.stdout.write(data);
	} catch {
		// Best-effort terminal hint; ignore unsupported terminal writes.
	}
}

export function CustomTextInput(props: CustomTextInputProps): React.JSX.Element {
	const {
		value,
		onChange,
		onSubmit,
		onEscape,
		placeholder = '',
		focus = true,
		showCursor = true,
		enableHistoryNavigation = true,
	} = props;
	const {setRawMode} = useStdin();

	const [cursorOffset, setCursorOffset] = useState<number>(value.length);
	const cursorRef = useRef<number>(value.length);
	const valueRef = useRef<string>(value);
	const pasteRef = useRef<PasteState>(initialPasteState);
	const rawModeOwnedRef = useRef<boolean>(false);
	const undoRef = useRef<UndoStack<UndoSnapshot>>(new UndoStack<UndoSnapshot>(150));
	const killRingRef = useRef<KillRing>(new KillRing(40));
	const historyRef = useRef(initialInputHistoryState);
	const lastActionRef = useRef<ActionType>('none');
	const lastYankRangeRef = useRef<Range | null>(null);

	useEffect(() => {
		valueRef.current = value;
		const clamped = Math.max(0, Math.min(cursorRef.current, value.length));
		cursorRef.current = clamped;
		setCursorOffset(clamped);
	}, [value]);

	useEffect(() => {
		if (!focus) {
			pasteRef.current = initialPasteState;
			if (rawModeOwnedRef.current) {
				writeTTY('\x1b[?2004l');
				setRawMode?.(false);
				rawModeOwnedRef.current = false;
			}
			return;
		}
		setRawMode?.(true);
		writeTTY('\x1b[?2004h');
		rawModeOwnedRef.current = true;

		return () => {
			if (!rawModeOwnedRef.current) {
				return;
			}
			writeTTY('\x1b[?2004l');
			setRawMode?.(false);
			rawModeOwnedRef.current = false;
		};
	}, [focus, setRawMode]);

	const applyNextValue = useCallback(
		(nextValue: string, nextCursor: number) => {
			const clamped = Math.max(0, Math.min(nextCursor, nextValue.length));
			const prevValue = valueRef.current;
			valueRef.current = nextValue;
			cursorRef.current = clamped;
			setCursorOffset(clamped);
			if (nextValue !== prevValue) {
				onChange(nextValue);
			}
		},
		[onChange],
	);

	const pushUndoSnapshot = useCallback(() => {
		undoRef.current.push({value: valueRef.current, cursor: cursorRef.current});
	}, []);

	const applyUndo = useCallback(() => {
		const snapshot = undoRef.current.pop();
		if (snapshot === undefined) {
			return;
		}
		applyNextValue(snapshot.value, snapshot.cursor);
		lastActionRef.current = 'none';
		lastYankRangeRef.current = null;
	}, [applyNextValue]);

	const setAction = useCallback((action: ActionType, yankRange: Range | null) => {
		lastActionRef.current = action;
		lastYankRangeRef.current = yankRange;
	}, []);

	const resetHistoryBrowsing = useCallback(() => {
		historyRef.current = resetInputHistoryCursor(historyRef.current);
	}, []);

	useInput(
		(input, key) => {
			if (key.escape) {
				if (valueRef.current !== '') {
					applyNextValue('', 0);
					resetHistoryBrowsing();
				}
				setAction('none', null);
				onEscape?.();
				return;
			}
			if (key.return) {
				historyRef.current = pushInputHistory(historyRef.current, valueRef.current);
				onSubmit();
				setAction('none', null);
				return;
			}
			if (enableHistoryNavigation && key.upArrow) {
				const result = navigateInputHistory(historyRef.current, valueRef.current, 'up');
				historyRef.current = result.state;
				if (result.changed) {
					applyNextValue(result.value, result.value.length);
				}
				return;
			}
			if (enableHistoryNavigation && key.downArrow) {
				const result = navigateInputHistory(historyRef.current, valueRef.current, 'down');
				historyRef.current = result.state;
				if (result.changed) {
					applyNextValue(result.value, result.value.length);
				}
				return;
			}
			if (key.leftArrow) {
				if (showCursor && cursorRef.current > 0) {
					setCursorOffset(cursorRef.current - 1);
					cursorRef.current -= 1;
				}
				setAction('none', null);
				return;
			}
			if (key.rightArrow) {
				if (showCursor && cursorRef.current < valueRef.current.length) {
					setCursorOffset(cursorRef.current + 1);
					cursorRef.current += 1;
				}
				setAction('none', null);
				return;
			}
			if (key.backspace || key.delete) {
				if (cursorRef.current <= 0) {
					return;
				}
				pushUndoSnapshot();
				const next = backspace({value: valueRef.current, cursor: cursorRef.current});
				applyNextValue(next.value, next.cursor);
				resetHistoryBrowsing();
				setAction('none', null);
				return;
			}

			if (key.ctrl && input === 'z') {
				applyUndo();
				return;
			}
			if (key.ctrl && input === 'u') {
				const result = deleteToLineStart({value: valueRef.current, cursor: cursorRef.current});
				if (result.deleted === '') {
					return;
				}
				pushUndoSnapshot();
				killRingRef.current.push(result.deleted, {
					prepend: true,
					accumulate: lastActionRef.current === 'kill',
				});
				applyNextValue(result.next.value, result.next.cursor);
				resetHistoryBrowsing();
				setAction('kill', null);
				return;
			}
			if (key.ctrl && input === 'k') {
				const result = deleteToLineEnd({value: valueRef.current, cursor: cursorRef.current});
				if (result.deleted === '') {
					return;
				}
				pushUndoSnapshot();
				killRingRef.current.push(result.deleted, {
					prepend: false,
					accumulate: lastActionRef.current === 'kill',
				});
				applyNextValue(result.next.value, result.next.cursor);
				resetHistoryBrowsing();
				setAction('kill', null);
				return;
			}
			if (key.ctrl && input === 'y') {
				const killed = killRingRef.current.peek();
				if (killed === undefined || killed === '') {
					return;
				}
				pushUndoSnapshot();
				const current = {value: valueRef.current, cursor: cursorRef.current};
				const next = insertText(current, killed);
				applyNextValue(next.value, next.cursor);
				resetHistoryBrowsing();
				setAction('yank', {start: current.cursor, end: current.cursor + killed.length});
				return;
			}
			if (key.meta && input.toLowerCase() === 'y') {
				const yankRange = lastYankRangeRef.current;
				if (lastActionRef.current !== 'yank' || yankRange === null || killRingRef.current.length <= 1) {
					return;
				}
				pushUndoSnapshot();
				const current = {value: valueRef.current, cursor: cursorRef.current};
				const removed = replaceRange(current, yankRange.start, yankRange.end, '');
				killRingRef.current.rotate();
				const nextText = killRingRef.current.peek() ?? '';
				const next = insertText(removed, nextText);
				applyNextValue(next.value, next.cursor);
				resetHistoryBrowsing();
				setAction('yank', {start: removed.cursor, end: removed.cursor + nextText.length});
				return;
			}
			if (key.tab) {
				const current = valueRef.current;
				const completed = completeCommandInput(current);
				if (completed === current) {
					return;
				}
				pushUndoSnapshot();
				applyNextValue(completed, completed.length);
				resetHistoryBrowsing();
				setAction('none', null);
				return;
			}
			if (key.ctrl || key.meta || input === '') {
				return;
			}

			const consumed = consumeBracketedPaste(pasteRef.current, input);
			pasteRef.current = consumed.state;

			if (consumed.chunks.length === 0) {
				return;
			}
			pushUndoSnapshot();
			let nextState = {value: valueRef.current, cursor: cursorRef.current};
			for (const chunk of consumed.chunks) {
				nextState = insertText(nextState, chunk.value);
			}
			applyNextValue(nextState.value, nextState.cursor);
			resetHistoryBrowsing();
			setAction('none', null);
		},
		{isActive: focus},
	);

	const renderedText = useMemo(() => {
		if (value === '') {
			if (!focus || !showCursor) {
				return chalk.gray(placeholder);
			}
			if (placeholder === '') {
				return chalk.inverse(' ');
			}
			return chalk.inverse(placeholder[0] ?? ' ') + chalk.gray(placeholder.slice(1));
		}

		if (!focus || !showCursor) {
			return value;
		}

		const before = value.slice(0, cursorOffset);
		const at = value.slice(cursorOffset, cursorOffset + 1);
		const after = value.slice(cursorOffset + 1);
		if (at === '') {
			return `${before}${chalk.inverse(' ')}`;
		}
		return `${before}${chalk.inverse(at)}${after}`;
	}, [cursorOffset, focus, placeholder, showCursor, value]);

	return <Text>{renderedText}</Text>;
}
