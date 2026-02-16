import React from 'react';
import {Box, Text} from 'ink';
import TextInput from 'ink-text-input';
import {ChatLine, SessionSummary} from '../types.js';

type HeaderBarProps = {
	serverUrl: string;
	sessionID: string;
	busy: boolean;
	scrollOffset: number;
};

export function HeaderBar(props: HeaderBarProps): React.JSX.Element {
	return (
		<Box marginBottom={1}>
			<Text color="cyan">tars-ui</Text>
			<Text>  server=</Text>
			<Text color="green">{props.serverUrl}</Text>
			<Text>  session=</Text>
			<Text color="yellow">{props.sessionID || '(new)'}</Text>
			<Text>  state=</Text>
			<Text color={props.busy ? 'yellow' : 'green'}>{props.busy ? 'streaming' : 'idle'}</Text>
			<Text>  scroll=</Text>
			<Text color="magenta">{props.scrollOffset}</Text>
		</Box>
	);
}

function roleLabel(role: ChatLine['role']): string {
	switch (role) {
	case 'assistant':
		return 'TARS';
	case 'error':
		return 'ERROR';
	case 'system':
		return 'SYSTEM';
	default:
		return 'YOU';
	}
}

function roleColor(role: ChatLine['role']): 'green' | 'red' | 'cyan' | 'white' {
	switch (role) {
	case 'assistant':
		return 'green';
	case 'error':
		return 'red';
	case 'system':
		return 'cyan';
	default:
		return 'white';
	}
}

type ChatPanelProps = {
	visibleMessages: ChatLine[];
	chatStart: number;
	assistantDraft: string;
	showDraft: boolean;
};

export function ChatPanel(props: ChatPanelProps): React.JSX.Element {
	return (
		<Box flexDirection="column" borderStyle="round" borderColor="cyan" paddingX={1} flexGrow={3} minHeight={20}>
			<Text color="cyan">Chat (PgUp/PgDn or Ctrl+U/Ctrl+D)</Text>
			{props.visibleMessages.map((message, idx) => (
				<Box key={`${props.chatStart + idx}-${message.role}`}>
					<Text color={roleColor(message.role)}>{roleLabel(message.role)} {'> '}</Text>
					<Text>{message.text}</Text>
				</Box>
			))}
			{props.assistantDraft !== '' && props.showDraft && (
				<Box>
					<Text color="green">TARS &gt; </Text>
					<Text>{props.assistantDraft}</Text>
				</Box>
			)}
		</Box>
	);
}

type StatusPanelProps = {
	visibleStatus: string[];
	visibleTools: string[];
	visibleNotifications: string[];
	visibleCronRuns: string[];
	notificationFilter: string;
	notificationUnreadCount: number;
	visibleDebug: string[];
	verbose: boolean;
};

export function StatusPanel(props: StatusPanelProps): React.JSX.Element {
	return (
		<Box flexDirection="column" borderStyle="round" borderColor="magenta" paddingX={1} flexGrow={2} minHeight={20}>
			<Text color="magenta">Status</Text>
			{props.visibleStatus.map((line, idx) => (
				<Text key={`status-${idx}`}>• {line}</Text>
			))}
			<Text color="yellow">Tools</Text>
			{props.visibleTools.map((line, idx) => (
				<Text key={`tool-${idx}`}>• {line}</Text>
			))}
			<Text color="cyan">
				Notifications (filter={props.notificationFilter}, unread={props.notificationUnreadCount})
			</Text>
			{props.visibleNotifications.map((line, idx) => (
				<Text key={`notify-${idx}`}>• {line}</Text>
			))}
			<Text color="green">Cron Runs</Text>
			{props.visibleCronRuns.map((line, idx) => (
				<Text key={`cron-runs-${idx}`}>• {line}</Text>
			))}
			{props.verbose && (
				<>
					<Text color="magentaBright">Debug</Text>
					{props.visibleDebug.map((line, idx) => (
						<Text key={`debug-${idx}`} dimColor>
							{line}
						</Text>
					))}
				</>
			)}
		</Box>
	);
}

type ResumePanelProps = {
	resumeCandidates: SessionSummary[] | null;
	resumeIndex: number;
};

export function ResumePanel(props: ResumePanelProps): React.JSX.Element | null {
	if (props.resumeCandidates === null) {
		return null;
	}

	return (
		<Box marginTop={1} flexDirection="column" borderStyle="round" borderColor="yellow" paddingX={1}>
			<Text color="yellow">Resume session (↑/↓, Enter)</Text>
			{props.resumeCandidates.map((session, idx) => (
				<Text key={session.id} color={idx === props.resumeIndex ? 'green' : 'white'}>
					{idx === props.resumeIndex ? '>' : ' '} {idx + 1}) {session.id} {session.title}
				</Text>
			))}
		</Box>
	);
}

type ChatInputProps = {
	input: string;
	onChange: (value: string) => void;
	onSubmit: () => void;
	busy: boolean;
	hasResumeCandidates: boolean;
};

function inputPlaceholder(busy: boolean, hasResumeCandidates: boolean): string {
	if (busy) {
		return 'waiting for response...';
	}
	if (hasResumeCandidates) {
		return 'Enter to select highlighted session (or type number)';
	}
	return 'Type message and press Enter';
}

export function ChatInput(props: ChatInputProps): React.JSX.Element {
	return (
		<Box marginTop={1}>
			<Text color="yellow">You &gt; </Text>
			<TextInput
				value={props.input}
				onChange={props.onChange}
				onSubmit={props.onSubmit}
				placeholder={inputPlaceholder(props.busy, props.hasResumeCandidates)}
				focus={!props.busy}
			/>
		</Box>
	);
}
