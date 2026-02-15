import {ChatSSEEvent, streamChat} from '../api/chat.js';
import {ChatUIAction} from './state.js';

export type SendChatContext = {
	serverUrl: string;
	sessionID: string;
	message: string;
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
		const messageText = err instanceof Error ? err.message : String(err);
		ctx.dispatchChat({type: 'stream_error', errorText: messageText});
		ctx.pushStatus(`error: ${messageText}`);
		ctx.pushDebug(`error: ${messageText}`);
	}
}
