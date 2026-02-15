import React, {useCallback, useMemo, useReducer, useState} from 'react';
import {Box, Text, render, useApp, useInput} from 'ink';
import TextInput from 'ink-text-input';
import {ChatSSEEvent, streamChat} from './api/chat.js';
import {createSession, exportSession, getHistory, getSession, listSessions, searchSessions} from './api/session.js';
import {getStatus, runCompact, runHeartbeatOnce} from './api/system.js';
import {chatUIReducer, initialChatUIState} from './chat/state.js';
import {parseArgs} from './cli/parseArgs.js';
import {parseInputCommand} from './commands/router.js';
import {SessionSummary} from './types.js';

const maxPanelLines = 200;
const chatPageSize = 20;

function appendBounded(lines: string[], next: string, max: number): string[] {
	const trimmed = next.trim();
	if (trimmed === '') {
		return lines;
	}
	const out = [...lines, trimmed];
	if (out.length <= max) {
		return out;
	}
	return out.slice(out.length - max);
}

function commandHelpText(): string {
	return 'Commands: /help, /sessions, /new [title], /resume {id}, /history, /export, /search {keyword}, /status, /compact, /heartbeat, /quit';
}

function requireSessionOrError(currentSession: string): string | null {
	if (currentSession.trim() === '') {
		return 'no active session. use /new or /resume {session_id}';
	}
	return null;
}

function padRight(value: string, width: number): string {
	const runes = Array.from(value);
	if (runes.length >= width) {
		return runes.slice(0, width).join('');
	}
	return value + ' '.repeat(width - runes.length);
}

function renderTable(headers: string[], rows: string[][]): string[] {
	const safeRows = rows.filter((r) => r.length === headers.length);
	const widths = headers.map((h, i) => {
		const bodyMax = safeRows.reduce((acc, row) => Math.max(acc, Array.from(row[i] ?? '').length), 0);
		return Math.max(Array.from(h).length, bodyMax, 4);
	});
	const header = headers.map((h, i) => padRight(h, widths[i] ?? 0)).join(' | ');
	const sep = widths.map((w) => '-'.repeat(w)).join('-|-');
	const body = safeRows.map((row) => row.map((c, i) => padRight(c, widths[i] ?? 0)).join(' | '));
	return [header, sep, ...body];
}

function truncate(value: string, max: number): string {
	const runes = Array.from(value);
	if (runes.length <= max) {
		return value;
	}
	if (max <= 3) {
		return runes.slice(0, max).join('');
	}
	return runes.slice(0, max - 3).join('') + '...';
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
	const [chatScrollOffset, setChatScrollOffset] = useState<number>(0);
	const [chatState, dispatchChat] = useReducer(chatUIReducer, initialChatUIState);

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
		const phase = (evt.phase ?? '').trim();
		if (phase === 'before_tool_call') {
			pushTool(`start ${evt.tool_name ?? ''}`.trim());
		}
		if (phase === 'after_tool_call') {
			pushTool(`done ${evt.tool_name ?? ''}`.trim());
		}
		if (phase === 'error') {
			pushTool(`error ${evt.message ?? evt.error ?? ''}`.trim());
		}
	}, [pushTool]);

	const executeCommand = useCallback(
		async (raw: string): Promise<void> => {
			const cmd = parseInputCommand(raw);
			switch (cmd.kind) {
			case 'noop':
				return;
			case 'chat':
				throw new Error('internal command router mismatch');
			case 'invalid':
				dispatchChat({type: 'append_message', message: {role: 'error', text: cmd.message}});
				return;
			case 'quit':
				exit();
				return;
			case 'help':
				pushSystemMessage(commandHelpText());
				return;
			case 'sessions': {
				const sessions = await listSessions(initial.serverUrl);
				if (sessions.length === 0) {
					pushSystemMessage('(no sessions)');
					return;
				}
				pushSystemTable(
					['ID', 'TITLE'],
					sessions.map((s) => [s.id, truncate(s.title, 48)]),
				);
				return;
			}
			case 'new': {
				const created = await createSession(initial.serverUrl, cmd.title);
				setSessionID(created.id);
				pushSystemMessage(`active session: ${created.id}`);
				return;
			}
			case 'resume': {
				await getSession(initial.serverUrl, cmd.sessionID);
				setSessionID(cmd.sessionID);
				pushSystemMessage(`resumed session: ${cmd.sessionID}`);
				setResumeCandidates(null);
				setResumeIndex(0);
				return;
			}
			case 'resume_select': {
				const sessions = await listSessions(initial.serverUrl);
				if (sessions.length === 0) {
					pushSystemMessage('(no sessions)');
					return;
				}
				setResumeCandidates(sessions);
				setResumeIndex(0);
				pushSystemMessage('Use ↑/↓ and Enter to select a session.');
				return;
			}
			case 'history': {
				const missing = requireSessionOrError(sessionID);
				if (missing) {
					dispatchChat({type: 'append_message', message: {role: 'error', text: missing}});
					return;
				}
				const history = await getHistory(initial.serverUrl, sessionID);
				if (history.length === 0) {
					pushSystemMessage('(no history)');
					return;
				}
				pushSystemTable(
					['TIME', 'ROLE', 'CONTENT'],
					history.map((m) => [m.timestamp, m.role, truncate(m.content.replace(/\s+/g, ' '), 64)]),
				);
				return;
			}
			case 'export': {
				const missing = requireSessionOrError(sessionID);
				if (missing) {
					dispatchChat({type: 'append_message', message: {role: 'error', text: missing}});
					return;
				}
				const markdown = await exportSession(initial.serverUrl, sessionID);
				pushSystemMessage(markdown);
				return;
			}
			case 'search': {
				const sessions = await searchSessions(initial.serverUrl, cmd.keyword);
				if (sessions.length === 0) {
					pushSystemMessage('(no sessions)');
					return;
				}
				pushSystemTable(
					['ID', 'TITLE'],
					sessions.map((s) => [s.id, truncate(s.title, 48)]),
				);
				return;
			}
			case 'status': {
				const status = await getStatus(initial.serverUrl);
				pushSystemMessage(`workspace=${status.workspace_dir} sessions=${status.session_count}`);
				return;
			}
			case 'compact': {
				const missing = requireSessionOrError(sessionID);
				if (missing) {
					dispatchChat({type: 'append_message', message: {role: 'error', text: missing}});
					return;
				}
				const message = await runCompact(initial.serverUrl, sessionID);
				pushSystemMessage(message);
				return;
			}
			case 'heartbeat': {
				const response = await runHeartbeatOnce(initial.serverUrl);
				pushSystemMessage(response);
				return;
			}
			}
		},
		[exit, initial.serverUrl, pushSystemMessage, pushSystemTable, sessionID],
	);

	const sendChat = useCallback(
		async (message: string): Promise<void> => {
			dispatchChat({type: 'append_message', message: {role: 'user', text: message}});
			dispatchChat({type: 'stream_start'});
			setResumeCandidates(null);
			setResumeIndex(0);
			setChatScrollOffset(0);

			try {
				const result = await streamChat({
					serverUrl: initial.serverUrl,
					sessionId: sessionID,
					message,
					onStatus: (status) => {
						pushStatus(status);
						pushDebug(`status: ${status}`);
					},
					onStatusEvent: handleStatusEvent,
					onDelta: (chunk) => {
						dispatchChat({type: 'stream_delta', chunk});
					},
					onDebug: pushDebug,
				});
				dispatchChat({type: 'stream_done', assistantText: result.assistantText});
				if (result.sessionId.trim() !== '') {
					setSessionID(result.sessionId.trim());
				}
			} catch (err) {
				const messageText = err instanceof Error ? err.message : String(err);
				dispatchChat({type: 'stream_error', errorText: messageText});
				pushStatus(`error: ${messageText}`);
				pushDebug(`error: ${messageText}`);
			}
		},
		[handleStatusEvent, initial.serverUrl, pushDebug, pushStatus, sessionID],
	);

	const selectResumeCandidate = useCallback(() => {
		if (resumeCandidates === null || resumeCandidates.length === 0) {
			return;
		}
		const selected = resumeCandidates[resumeIndex] ?? resumeCandidates[0];
		if (!selected) {
			return;
		}
		setSessionID(selected.id);
		pushSystemMessage(`resumed session: ${selected.id}`);
		setResumeCandidates(null);
		setResumeIndex(0);
	}, [pushSystemMessage, resumeCandidates, resumeIndex]);

	const submit = useCallback(async (): Promise<void> => {
		const line = input.trim();
		if (line === '' && resumeCandidates === null) {
			return;
		}
		if (chatState.busy) {
			return;
		}
		setInput('');

		if (resumeCandidates !== null) {
			if (line === '') {
				selectResumeCandidate();
				return;
			}
			const choice = Number.parseInt(line, 10);
			if (Number.isNaN(choice) || choice < 1 || choice > resumeCandidates.length) {
				dispatchChat({type: 'append_message', message: {role: 'error', text: 'invalid selection'}});
				return;
			}
			const session = resumeCandidates[choice - 1]!;
			setSessionID(session.id);
			setResumeCandidates(null);
			setResumeIndex(0);
			pushSystemMessage(`resumed session: ${session.id}`);
			return;
		}

		if (line.startsWith('/')) {
			try {
				await executeCommand(line);
			} catch (err) {
				const message = err instanceof Error ? err.message : String(err);
				dispatchChat({type: 'append_message', message: {role: 'error', text: message}});
				pushDebug(`command error: ${message}`);
			}
			return;
		}

		await sendChat(line);
	}, [chatState.busy, executeCommand, input, pushDebug, pushSystemMessage, resumeCandidates, selectResumeCandidate, sendChat]);

	useInput((key, inputState) => {
		if (inputState.ctrl && key === 'c') {
			exit();
			return;
		}

		if (resumeCandidates !== null && resumeCandidates.length > 0) {
			if (inputState.upArrow) {
				setResumeIndex((prev) => (prev - 1 + resumeCandidates.length) % resumeCandidates.length);
				return;
			}
			if (inputState.downArrow) {
				setResumeIndex((prev) => (prev + 1) % resumeCandidates.length);
				return;
			}
		}

		if (inputState.pageUp || (inputState.ctrl && key === 'u')) {
			setChatScrollOffset((prev) => prev + chatPageSize);
			return;
		}
		if (inputState.pageDown || (inputState.ctrl && key === 'd')) {
			setChatScrollOffset((prev) => Math.max(0, prev - chatPageSize));
		}
	});

	const maxOffset = useMemo(() => Math.max(0, chatState.messages.length - chatPageSize), [chatState.messages.length]);
	const effectiveOffset = Math.min(chatScrollOffset, maxOffset);
	const chatEnd = chatState.messages.length - effectiveOffset;
	const chatStart = Math.max(0, chatEnd - chatPageSize);
	const visibleMessages = useMemo(() => chatState.messages.slice(chatStart, chatEnd), [chatEnd, chatStart, chatState.messages]);
	const visibleStatus = useMemo(() => statusLines.slice(-20), [statusLines]);
	const visibleTools = useMemo(() => toolLines.slice(-20), [toolLines]);
	const visibleDebug = useMemo(() => debugLines.slice(-20), [debugLines]);

	return (
		<Box flexDirection="column">
			<Box marginBottom={1}>
				<Text color="cyan">tars-ui</Text>
				<Text>  server=</Text>
				<Text color="green">{initial.serverUrl}</Text>
				<Text>  session=</Text>
				<Text color="yellow">{sessionID || '(new)'}</Text>
				<Text>  state=</Text>
				<Text color={chatState.busy ? 'yellow' : 'green'}>{chatState.busy ? 'streaming' : 'idle'}</Text>
				<Text>  scroll=</Text>
				<Text color="magenta">{effectiveOffset}</Text>
			</Box>

			<Box>
				<Box flexDirection="column" borderStyle="round" borderColor="cyan" paddingX={1} flexGrow={3} minHeight={20}>
					<Text color="cyan">Chat (PgUp/PgDn or Ctrl+U/Ctrl+D)</Text>
					{visibleMessages.map((m, idx) => (
						<Box key={`${chatStart + idx}-${m.role}`}>
							<Text color={m.role === 'assistant' ? 'green' : m.role === 'error' ? 'red' : m.role === 'system' ? 'cyan' : 'white'}>
								{m.role === 'assistant' ? 'TARS' : m.role === 'error' ? 'ERROR' : m.role === 'system' ? 'SYSTEM' : 'YOU'} {'> '}
							</Text>
							<Text>{m.text}</Text>
						</Box>
					))}
					{chatState.assistantDraft !== '' && effectiveOffset === 0 && (
						<Box>
							<Text color="green">TARS &gt; </Text>
							<Text>{chatState.assistantDraft}</Text>
						</Box>
					)}
				</Box>

				<Box width={1} />

				<Box flexDirection="column" borderStyle="round" borderColor="magenta" paddingX={1} flexGrow={2} minHeight={20}>
					<Text color="magenta">Status</Text>
					{visibleStatus.map((line, idx) => (
						<Text key={`status-${idx}`}>• {line}</Text>
					))}
					<Text color="yellow">Tools</Text>
					{visibleTools.map((line, idx) => (
						<Text key={`tool-${idx}`}>• {line}</Text>
					))}
					{initial.verbose && (
						<>
							<Text color="magentaBright">Debug</Text>
							{visibleDebug.map((line, idx) => (
								<Text key={`debug-${idx}`} dimColor>
									{line}
								</Text>
							))}
						</>
					)}
				</Box>
			</Box>

			{resumeCandidates !== null && (
				<Box marginTop={1} flexDirection="column" borderStyle="round" borderColor="yellow" paddingX={1}>
					<Text color="yellow">Resume session (↑/↓, Enter)</Text>
					{resumeCandidates.map((s, idx) => (
						<Text key={s.id} color={idx === resumeIndex ? 'green' : 'white'}>
							{idx === resumeIndex ? '>' : ' '} {idx + 1}) {s.id} {s.title}
						</Text>
					))}
				</Box>
			)}

			<Box marginTop={1}>
				<Text color="yellow">You &gt; </Text>
				<TextInput
					value={input}
					onChange={setInput}
					onSubmit={() => {
						void submit();
					}}
					placeholder={
						chatState.busy
							? 'waiting for response...'
							: resumeCandidates !== null
								? 'Enter to select highlighted session (or type number)'
								: 'Type message and press Enter'
					}
					focus={!chatState.busy}
				/>
			</Box>
		</Box>
	);
}

render(<App />);

