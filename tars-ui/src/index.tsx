import React, {useCallback, useEffect, useMemo, useReducer, useRef, useState} from 'react';
import {Box, render, useApp, useInput} from 'ink';
import {ChatSSEEvent} from './api/chat.js';
import {NotificationEvent, watchNotifications} from './api/events.js';
import {executeInputCommand} from './chat/commandExecutor.js';
import {sendChatMessage} from './chat/sendChat.js';
import {chatUIReducer, initialChatUIState} from './chat/state.js';
import {submitInput} from './chat/submit.js';
import {computeChatWindow, nextChatScrollOffset, nextResumeIndex, resolveKeyAction, tailLines, toolLinesFromStatusEvent} from './chat/view.js';
import {parseArgs} from './cli/parseArgs.js';
import {NotificationFilter, NotificationItem, SessionSummary} from './types.js';
import {appendBounded, renderTable, truncate} from './ui/format.js';
import {ChatInput, ChatPanel, HeaderBar, ResumePanel, StatusPanel} from './ui/panels.js';

const maxPanelLines = 200;
const chatPageSize = 20;
const maxNotificationItems = 300;
const notificationPreviewLines = 8;

function matchesNotificationFilter(item: NotificationItem, filter: NotificationFilter): boolean {
	if (filter === 'all') {
		return true;
	}
	if (filter === 'error') {
		return item.severity === 'error';
	}
	return item.category === filter;
}

function notificationLine(item: NotificationItem): string {
	const category = item.category.trim() === '' ? 'event' : item.category.trim();
	const severity = item.severity.trim() === '' ? 'info' : item.severity.trim();
	const title = item.title.trim() === '' ? '(untitled)' : item.title.trim();
	const message = item.message.trim();
	const head = `[${category}/${severity}] ${truncate(title, 28)}`;
	if (message === '') {
		return head;
	}
	return `${head} | ${truncate(message, 48)}`;
}

function App(): React.JSX.Element {
	const initial = useMemo(() => parseArgs(process.argv.slice(2)), []);
	const {exit} = useApp();

	const [sessionID, setSessionID] = useState<string>(initial.sessionId);
	const [input, setInput] = useState<string>('');
	const [statusLines, setStatusLines] = useState<string[]>([]);
	const [toolLines, setToolLines] = useState<string[]>([]);
	const [debugLines, setDebugLines] = useState<string[]>([]);
	const [resumeCandidates, setResumeCandidates] = useState<SessionSummary[] | null>(null);
	const [resumeIndex, setResumeIndex] = useState<number>(0);
	const [notificationFilter, setNotificationFilter] = useState<NotificationFilter>('all');
	const [notifications, setNotifications] = useState<NotificationItem[]>([]);
	const [cronRunsPreview, setCronRunsPreview] = useState<string[]>([]);
	const [lastSeenNotificationID, setLastSeenNotificationID] = useState<number>(0);
	const [chatScrollOffset, setChatScrollOffset] = useState<number>(0);
	const [chatState, dispatchChat] = useReducer(chatUIReducer, initialChatUIState);
	const nextNotificationID = useRef<number>(1);

	const pushStatus = useCallback((line: string) => {
		setStatusLines((prev) => appendBounded(prev, line, maxPanelLines));
	}, []);

	const pushTool = useCallback((line: string) => {
		setToolLines((prev) => appendBounded(prev, line, maxPanelLines));
	}, []);

	const pushDebug = useCallback(
		(line: string) => {
			if (!initial.verbose) {
				return;
			}
			setDebugLines((prev) => appendBounded(prev, line, maxPanelLines));
		},
		[initial.verbose],
	);

	const pushSystemMessage = useCallback((text: string) => {
		dispatchChat({type: 'append_message', message: {role: 'system', text}});
	}, []);

	const pushSystemTable = useCallback(
		(headers: string[], rows: string[][]) => {
			for (const line of renderTable(headers, rows)) {
				pushSystemMessage(line);
			}
		},
		[pushSystemMessage],
	);

	const handleStatusEvent = useCallback((evt: ChatSSEEvent) => {
		for (const line of toolLinesFromStatusEvent(evt)) {
			pushTool(line);
		}
	}, [pushTool]);

	const handleNotificationEvent = useCallback((evt: NotificationEvent) => {
		const category = (evt.category ?? '').trim();
		const severity = (evt.severity ?? '').trim();
		const title = (evt.title ?? '').trim();
		const message = (evt.message ?? '').trim();
		const timestamp = (evt.timestamp ?? '').trim();
		setNotifications((prev) => {
			const item: NotificationItem = {
				id: nextNotificationID.current++,
				category,
				severity,
				title,
				message,
				timestamp,
			};
			const next = [...prev, item];
			if (next.length <= maxNotificationItems) {
				return next;
			}
			return next.slice(next.length - maxNotificationItems);
		});
		const summary = [title, message].filter((v) => v !== '').join(' | ');
		if (summary !== '') {
			pushStatus(`notify ${category !== '' ? category : 'event'}: ${summary}`);
			pushSystemMessage(`[notify] ${summary}`);
		}
	}, [pushStatus, pushSystemMessage]);

	const executeCommand = useCallback(
		async (raw: string): Promise<void> => {
			await executeInputCommand({
				raw,
				serverUrl: initial.serverUrl,
				sessionID,
				pushSystemMessage,
				pushSystemTable,
				pushErrorMessage: (text) => {
					dispatchChat({type: 'append_message', message: {role: 'error', text}});
				},
				setSessionID,
				setResumeCandidates,
				setResumeIndex,
				setNotificationFilter,
				getNotificationFilter: () => notificationFilter,
				getNotifications: () => notifications,
				clearNotifications: () => {
					setNotifications([]);
					setLastSeenNotificationID(nextNotificationID.current - 1);
				},
				markNotificationsSeen: () => {
					setLastSeenNotificationID((prev) => Math.max(prev, nextNotificationID.current - 1));
				},
				setCronRunsPreview,
				exit,
			});
		},
		[exit, initial.serverUrl, notificationFilter, notifications, pushSystemMessage, pushSystemTable, sessionID],
	);

	const sendChat = useCallback(
		async (message: string): Promise<void> => {
			await sendChatMessage({
				serverUrl: initial.serverUrl,
				sessionID,
				message,
				dispatchChat,
				clearResumeSelection: () => {
					setResumeCandidates(null);
					setResumeIndex(0);
				},
				resetChatScroll: () => {
					setChatScrollOffset(0);
				},
				pushStatus,
				pushDebug,
				handleStatusEvent,
				setSessionID,
			});
		},
		[handleStatusEvent, initial.serverUrl, pushDebug, pushStatus, sessionID],
	);

	const submit = useCallback(async (): Promise<void> => {
		await submitInput({
			input,
			busy: chatState.busy,
			resumeCandidates,
			resumeIndex,
			setInput,
			setSessionID,
			setResumeCandidates,
			setResumeIndex,
			pushSystemMessage,
			pushErrorMessage: (text) => {
				dispatchChat({type: 'append_message', message: {role: 'error', text}});
			},
			pushDebug,
			executeCommand,
			sendChat,
		});
	}, [chatState.busy, executeCommand, input, pushDebug, pushSystemMessage, resumeCandidates, resumeIndex, sendChat]);

	useEffect(() => {
		const controller = new AbortController();
		let closed = false;
		const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));
		const run = async () => {
			while (!closed) {
				try {
					await watchNotifications({
						serverUrl: initial.serverUrl,
						onEvent: handleNotificationEvent,
						onDebug: pushDebug,
						signal: controller.signal,
					});
					if (!closed) {
						pushStatus('notification stream closed; reconnecting...');
					}
				} catch (error) {
					if (closed || controller.signal.aborted) {
						return;
					}
					pushStatus(`notification stream error: ${String(error)}`);
				}
				await sleep(1500);
			}
		};
		void run();
		return () => {
			closed = true;
			controller.abort();
		};
	}, [handleNotificationEvent, initial.serverUrl, pushDebug, pushStatus]);

	useInput((key, inputState) => {
		const action = resolveKeyAction(key, inputState, resumeCandidates !== null && resumeCandidates.length > 0);
		if (action === 'exit') {
			exit();
			return;
		}
		if (action === 'resume_up') {
			setResumeIndex((prev) => nextResumeIndex(prev, resumeCandidates?.length ?? 0, 'up'));
			return;
		}
		if (action === 'resume_down') {
			setResumeIndex((prev) => nextResumeIndex(prev, resumeCandidates?.length ?? 0, 'down'));
			return;
		}
		if (action === 'chat_page_up') {
			setChatScrollOffset((prev) => nextChatScrollOffset(prev, chatPageSize, 'up'));
			return;
		}
		if (action === 'chat_page_down') {
			setChatScrollOffset((prev) => nextChatScrollOffset(prev, chatPageSize, 'down'));
		}
	});

	const window = useMemo(() => computeChatWindow(chatState.messages.length, chatPageSize, chatScrollOffset), [chatScrollOffset, chatState.messages.length]);
	const effectiveOffset = window.effectiveOffset;
	const chatEnd = window.chatEnd;
	const chatStart = window.chatStart;
	const visibleMessages = useMemo(() => chatState.messages.slice(chatStart, chatEnd), [chatEnd, chatStart, chatState.messages]);
	const visibleStatus = useMemo(() => tailLines(statusLines, 20), [statusLines]);
	const visibleTools = useMemo(() => tailLines(toolLines, 20), [toolLines]);
	const filteredNotifications = useMemo(
		() => notifications.filter((item) => matchesNotificationFilter(item, notificationFilter)),
		[notificationFilter, notifications],
	);
	const notificationUnreadCount = useMemo(
		() => filteredNotifications.filter((item) => item.id > lastSeenNotificationID).length,
		[filteredNotifications, lastSeenNotificationID],
	);
	const visibleNotifications = useMemo(
		() => tailLines(filteredNotifications.map(notificationLine), notificationPreviewLines),
		[filteredNotifications],
	);
	const visibleCronRuns = useMemo(() => tailLines(cronRunsPreview, notificationPreviewLines), [cronRunsPreview]);
	const visibleDebug = useMemo(() => tailLines(debugLines, 20), [debugLines]);

	return (
		<Box flexDirection="column">
			<HeaderBar serverUrl={initial.serverUrl} sessionID={sessionID} busy={chatState.busy} scrollOffset={effectiveOffset} />

			<Box>
				<ChatPanel visibleMessages={visibleMessages} chatStart={chatStart} assistantDraft={chatState.assistantDraft} showDraft={effectiveOffset === 0} />

				<Box width={1} />

				<StatusPanel
					visibleStatus={visibleStatus}
					visibleTools={visibleTools}
					visibleNotifications={visibleNotifications}
					visibleCronRuns={visibleCronRuns}
					notificationFilter={notificationFilter}
					notificationUnreadCount={notificationUnreadCount}
					visibleDebug={visibleDebug}
					verbose={initial.verbose}
				/>
			</Box>

			<ResumePanel resumeCandidates={resumeCandidates} resumeIndex={resumeIndex} />

			<ChatInput
				input={input}
				onChange={setInput}
				onSubmit={() => {
					void submit();
				}}
				busy={chatState.busy}
				hasResumeCandidates={resumeCandidates !== null}
			/>
		</Box>
	);
}

render(<App />);
