import {ChatLine} from '../types.js';

const maxChatLines = 200;

export type ChatUIState = {
	messages: ChatLine[];
	assistantDraft: string;
	busy: boolean;
};

export type ChatUIAction =
	| {type: 'append_message'; message: ChatLine}
	| {type: 'stream_start'}
	| {type: 'stream_delta'; chunk: string}
	| {type: 'stream_done'; assistantText: string}
	| {type: 'stream_cancel'}
	| {type: 'stream_error'; errorText: string};

export const initialChatUIState: ChatUIState = {
	messages: [],
	assistantDraft: '',
	busy: false,
};

function appendBounded(messages: ChatLine[], message: ChatLine): ChatLine[] {
	const out = [...messages, message];
	if (out.length <= maxChatLines) {
		return out;
	}
	return out.slice(out.length - maxChatLines);
}

export function chatUIReducer(state: ChatUIState, action: ChatUIAction): ChatUIState {
	switch (action.type) {
	case 'append_message':
		return {
			...state,
			messages: appendBounded(state.messages, action.message),
		};
	case 'stream_start':
		return {
			...state,
			busy: true,
			assistantDraft: '',
		};
	case 'stream_delta':
		return {
			...state,
			assistantDraft: state.assistantDraft + action.chunk,
		};
	case 'stream_done': {
		const text = action.assistantText.trim() !== '' ? action.assistantText : state.assistantDraft;
		const nextMessages = text.trim() === '' ? state.messages : appendBounded(state.messages, {role: 'assistant', text});
		return {
			messages: nextMessages,
			assistantDraft: '',
			busy: false,
		};
	}
	case 'stream_cancel': {
		const text = state.assistantDraft.trim();
		const nextMessages = text === '' ? state.messages : appendBounded(state.messages, {role: 'assistant', text: state.assistantDraft});
		return {
			messages: nextMessages,
			assistantDraft: '',
			busy: false,
		};
	}
	case 'stream_error':
		return {
			messages: appendBounded(state.messages, {role: 'error', text: action.errorText}),
			assistantDraft: '',
			busy: false,
		};
	default:
		return state;
	}
}
