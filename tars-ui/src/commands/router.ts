export type Command =
	| {kind: 'noop'}
	| {kind: 'chat'; message: string}
	| {kind: 'help'}
	| {kind: 'sessions'}
	| {kind: 'new'; title: string}
	| {kind: 'resume'; sessionID: string}
	| {kind: 'resume_select'}
	| {kind: 'history'}
	| {kind: 'export'}
	| {kind: 'search'; keyword: string}
	| {kind: 'status'}
	| {kind: 'compact'}
	| {kind: 'heartbeat'}
	| {kind: 'quit'}
	| {kind: 'invalid'; message: string};

function normalizeCommandPrefix(line: string): string {
	if (line === '') {
		return line;
	}
	const first = Array.from(line)[0] ?? '';
	if (first === '\\' || first === '＼' || first === '₩' || first === '￦' || first === '／') {
		return `/${line.slice(first.length)}`;
	}
	return line;
}

function trimPrefix(raw: string, command: string): string {
	return raw.slice(command.length).trim();
}

export function parseInputCommand(raw: string): Command {
	const line = normalizeCommandPrefix(raw.trim());
	if (line === '') {
		return {kind: 'noop'};
	}
	if (!line.startsWith('/')) {
		return {kind: 'chat', message: line};
	}

	const fields = line.split(/\s+/);
	const head = fields[0] ?? '';

	switch (head) {
	case '/help':
		return {kind: 'help'};
	case '/sessions':
		return {kind: 'sessions'};
	case '/new': {
		const title = trimPrefix(line, '/new') || 'chat';
		return {kind: 'new', title};
	}
	case '/resume': {
		if (fields.length >= 2) {
			return {kind: 'resume', sessionID: fields[1]!.trim()};
		}
		return {kind: 'resume_select'};
	}
	case '/history':
		return {kind: 'history'};
	case '/export':
		return {kind: 'export'};
	case '/search': {
		const keyword = trimPrefix(line, '/search');
		if (keyword === '') {
			return {kind: 'invalid', message: 'usage: /search {keyword}'};
		}
		return {kind: 'search', keyword};
	}
	case '/status':
		return {kind: 'status'};
	case '/compact':
		return {kind: 'compact'};
	case '/heartbeat':
		return {kind: 'heartbeat'};
	case '/exit':
	case '/quit':
		return {kind: 'quit'};
	default:
		return {kind: 'invalid', message: `unknown command: ${head}`};
	}
}
