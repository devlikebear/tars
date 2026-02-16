import {ChatSSEEvent} from '../api/chat.js';

export type KeyInputState = {
	ctrl: boolean;
	upArrow: boolean;
	downArrow: boolean;
	pageUp: boolean;
	pageDown: boolean;
};

export type KeyAction = 'none' | 'exit' | 'resume_up' | 'resume_down' | 'chat_page_up' | 'chat_page_down';

function toolHeader(phase: string, toolName: string, toolCallID: string): string | null {
	const name = toolName.trim();
	const callID = toolCallID.trim();
	const suffix = callID !== '' ? ` #${callID}` : '';
	if (phase === 'before_tool_call') {
		return `start ${name}${suffix}`.trim();
	}
	if (phase === 'after_tool_call') {
		return `done ${name}${suffix}`.trim();
	}
	return null;
}

export function toolLinesFromStatusEvent(evt: ChatSSEEvent): string[] {
	const phase = (evt.phase ?? '').trim();
	const toolName = (evt.tool_name ?? '').trim();
	const toolCallID = (evt.tool_call_id ?? '').trim();
	const argsPreview = (evt.tool_args_preview ?? '').trim();
	const resultPreview = (evt.tool_result_preview ?? '').trim();

	const lines: string[] = [];
	const header = toolHeader(phase, toolName, toolCallID);
	if (header !== null) {
		lines.push(header);
	}
	if (phase === 'before_tool_call' && argsPreview !== '') {
		lines.push(`args ${argsPreview}`);
	}
	if (phase === 'after_tool_call' && resultPreview !== '') {
		lines.push(`result ${resultPreview}`);
	}
	if (phase === 'error') {
		lines.push(`error ${evt.message ?? evt.error ?? ''}`.trim());
	}
	return lines;
}

export function resolveKeyAction(
	key: string,
	inputState: KeyInputState,
	hasResumeCandidates: boolean,
	inputValue = '',
): KeyAction {
	if (inputState.ctrl && key === 'c') {
		return 'exit';
	}
	if (hasResumeCandidates && inputState.upArrow) {
		return 'resume_up';
	}
	if (hasResumeCandidates && inputState.downArrow) {
		return 'resume_down';
	}
	if (inputState.pageUp || (inputState.ctrl && key === 'u' && inputValue.trim() === '')) {
		return 'chat_page_up';
	}
	if (inputState.pageDown || (inputState.ctrl && key === 'd')) {
		return 'chat_page_down';
	}
	return 'none';
}

export function nextResumeIndex(current: number, total: number, direction: 'up' | 'down'): number {
	if (total <= 0) {
		return current;
	}
	if (direction === 'up') {
		return (current - 1 + total) % total;
	}
	return (current + 1) % total;
}

export function nextChatScrollOffset(current: number, pageSize: number, direction: 'up' | 'down'): number {
	if (direction === 'up') {
		return current + pageSize;
	}
	return Math.max(0, current - pageSize);
}

export type ChatWindow = {
	maxOffset: number;
	effectiveOffset: number;
	chatStart: number;
	chatEnd: number;
};

export function computeChatWindow(messageCount: number, pageSize: number, scrollOffset: number): ChatWindow {
	const maxOffset = Math.max(0, messageCount - pageSize);
	const effectiveOffset = Math.min(scrollOffset, maxOffset);
	const chatEnd = messageCount - effectiveOffset;
	const chatStart = Math.max(0, chatEnd - pageSize);
	return {maxOffset, effectiveOffset, chatStart, chatEnd};
}

export function tailLines(lines: string[], count: number): string[] {
	return lines.slice(-count);
}
