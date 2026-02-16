import {ChatSSEEvent, streamChat} from '../api/chat.js';
import {ChatUIAction} from './state.js';

export type SendChatContext = {
	serverUrl: string;
	sessionID: string;
	message: string;
	abortSignal?: AbortSignal;
	dispatchChat: (action: ChatUIAction) => void;
	clearResumeSelection: () => void;
	resetChatScroll: () => void;
	pushStatus: (line: string) => void;
	pushDebug: (line: string) => void;
	handleStatusEvent: (evt: ChatSSEEvent) => void;
	setSessionID: (sessionID: string) => void;
};

export type SendChatAPIs = {
	streamChat: typeof streamChat;
};

const defaultAPIs: SendChatAPIs = {streamChat};

function isAbortError(error: unknown): boolean {
	if (error instanceof DOMException && error.name === 'AbortError') {
		return true;
	}
	if (error instanceof Error) {
		const message = `${error.name} ${error.message}`.toLowerCase();
		return message.includes('abort');
	}
	return false;
}

export async function sendChatMessage(ctx: SendChatContext, apis: SendChatAPIs = defaultAPIs): Promise<void> {
	ctx.dispatchChat({type: 'append_message', message: {role: 'user', text: ctx.message}});
	ctx.dispatchChat({type: 'stream_start'});
	ctx.clearResumeSelection();
	ctx.resetChatScroll();

	try {
		const result = await apis.streamChat({
			serverUrl: ctx.serverUrl,
			sessionId: ctx.sessionID,
			message: ctx.message,
			signal: ctx.abortSignal,
			onStatus: (status) => {
				ctx.pushStatus(status);
				ctx.pushDebug(`status: ${status}`);
			},
			onStatusEvent: ctx.handleStatusEvent,
			onDelta: (chunk) => {
				ctx.dispatchChat({type: 'stream_delta', chunk});
			},
			onDebug: ctx.pushDebug,
		});
		ctx.dispatchChat({type: 'stream_done', assistantText: result.assistantText});
		const nextSessionID = result.sessionId.trim();
		if (nextSessionID !== '') {
			ctx.setSessionID(nextSessionID);
		}
	} catch (err) {
		if (ctx.abortSignal?.aborted || isAbortError(err)) {
			ctx.dispatchChat({type: 'stream_cancel'});
			ctx.pushStatus('stopped by user');
			ctx.pushDebug('stream canceled by user');
			return;
		}
		const messageText = err instanceof Error ? err.message : String(err);
		ctx.dispatchChat({type: 'stream_error', errorText: messageText});
		ctx.pushStatus(`error: ${messageText}`);
		ctx.pushDebug(`error: ${messageText}`);
	}
}
