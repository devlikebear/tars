import React, {useCallback, useMemo, useReducer, useState} from 'react';
import {Box, Text, render, useApp, useInput} from 'ink';
import TextInput from 'ink-text-input';
import {streamChat} from './api/chat.js';
import {createSession, exportSession, getHistory, getSession, listSessions, searchSessions} from './api/session.js';
import {getStatus, runCompact, runHeartbeatOnce} from './api/system.js';
import {chatUIReducer, initialChatUIState} from './chat/state.js';
import {parseArgs} from './cli/parseArgs.js';
import {parseInputCommand} from './commands/router.js';
import {SessionSummary} from './types.js';

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

function App(): React.JSX.Element {
	const initial = useMemo(() => parseArgs(process.argv.slice(2)), []);
	const {exit} = useApp();

	const [sessionID, setSessionID] = useState<string>(initial.sessionId);
	const [input, setInput] = useState<string>('');
	const [statusLines, setStatusLines] = useState<string[]>([]);
	const [debugLines, setDebugLines] = useState<string[]>([]);
	const [resumeCandidates, setResumeCandidates] = useState<SessionSummary[] | null>(null);
	const [chatState, dispatchChat] = useReducer(chatUIReducer, initialChatUIState);

	const pushStatus = useCallback((line: string) => {
		setStatusLines((prev) => appendBounded(prev, line, 200));
	}, []);

	const pushDebug = useCallback(
		(line: string) => {
			if (!initial.verbose) {
				return;
			}
			setDebugLines((prev) => appendBounded(prev, line, 200));
		},
		[initial.verbose],
	);

	const pushSystemMessage = useCallback((text: string) => {
		dispatchChat({type: 'append_message', message: {role: 'system', text}});
	}, []);

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
				for (const s of sessions) {
					pushSystemMessage(`${s.id}\t${s.title}`);
				}
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
				return;
			}
			case 'resume_select': {
				const sessions = await listSessions(initial.serverUrl);
				if (sessions.length === 0) {
					pushSystemMessage('(no sessions)');
					return;
				}
				setResumeCandidates(sessions);
				pushSystemMessage('Select session:');
				sessions.forEach((s, i) => {
					pushSystemMessage(`${i + 1}) ${s.id}\t${s.title}`);
				});
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
				for (const m of history) {
					pushSystemMessage(`${m.timestamp} [${m.role}] ${m.content}`);
				}
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
				for (const s of sessions) {
					pushSystemMessage(`${s.id}\t${s.title}`);
				}
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
		[exit, initial.serverUrl, pushSystemMessage, sessionID],
	);

	const sendChat = useCallback(
		async (message: string): Promise<void> => {
			dispatchChat({type: 'append_message', message: {role: 'user', text: message}});
			dispatchChat({type: 'stream_start'});
			setResumeCandidates(null);

			try {
				const result = await streamChat({
					serverUrl: initial.serverUrl,
					sessionId: sessionID,
					message,
					onStatus: (status) => {
						pushStatus(status);
						pushDebug(`status: ${status}`);
					},
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
		[initial.serverUrl, pushDebug, pushStatus, sessionID],
	);

	const submit = useCallback(async (): Promise<void> => {
		const line = input.trim();
		if (line === '' || chatState.busy) {
			return;
		}
		setInput('');

		if (resumeCandidates !== null && !line.startsWith('/')) {
			const choice = Number.parseInt(line, 10);
			if (Number.isNaN(choice) || choice < 1 || choice > resumeCandidates.length) {
				dispatchChat({type: 'append_message', message: {role: 'error', text: 'invalid selection'}});
				return;
			}
			const session = resumeCandidates[choice - 1]!;
			setSessionID(session.id);
			setResumeCandidates(null);
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
	}, [chatState.busy, executeCommand, input, pushDebug, pushSystemMessage, resumeCandidates, sendChat]);

	useInput((key, inputState) => {
		if (inputState.ctrl && key === 'c') {
			exit();
		}
	});

	const visibleMessages = useMemo(() => chatState.messages.slice(-30), [chatState.messages]);
	const visibleStatus = useMemo(() => statusLines.slice(-20), [statusLines]);
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
			</Box>

			<Box>
				<Box flexDirection="column" borderStyle="round" borderColor="cyan" paddingX={1} flexGrow={3} minHeight={20}>
					<Text color="cyan">Chat</Text>
					{visibleMessages.map((m, idx) => (
						<Box key={`${idx}-${m.role}`}>
							<Text color={m.role === 'assistant' ? 'green' : m.role === 'error' ? 'red' : m.role === 'system' ? 'cyan' : 'white'}>
								{m.role === 'assistant' ? 'TARS' : m.role === 'error' ? 'ERROR' : m.role === 'system' ? 'SYSTEM' : 'YOU'} {'> '}
							</Text>
							<Text>{m.text}</Text>
						</Box>
					))}
					{chatState.assistantDraft !== '' && (
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
								? 'Select session number'
								: 'Type message and press Enter'
					}
					focus={!chatState.busy}
				/>
			</Box>
		</Box>
	);
}

render(<App />);

