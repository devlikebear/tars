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
	| {kind: 'cron_list'}
	| {kind: 'cron_add'; schedule: string; prompt: string}
	| {kind: 'cron_run'; jobID: string}
	| {kind: 'cron_delete'; jobID: string}
	| {kind: 'cron_enable'; jobID: string}
	| {kind: 'cron_disable'; jobID: string}
	| {kind: 'notify_list'}
	| {kind: 'notify_filter'; filter: 'all' | 'cron' | 'heartbeat' | 'error'}
	| {kind: 'notify_open'; index: number}
	| {kind: 'notify_clear'}
	| {kind: 'quit'}
	| {kind: 'invalid'; message: string};

const alternateCommandPrefixes = new Set(['\\', '＼', '₩', '￦', '／']);

function normalizeCommandPrefix(line: string): string {
	if (line === '') {
		return line;
	}
	const first = Array.from(line)[0] ?? '';
	if (alternateCommandPrefixes.has(first)) {
		return `/${line.slice(first.length)}`;
	}
	return line;
}

function trimPrefix(raw: string, command: string): string {
	return raw.slice(command.length).trim();
}

function parseSimpleSlashCommand(head: string): Command | null {
	switch (head) {
	case '/help':
		return {kind: 'help'};
	case '/sessions':
		return {kind: 'sessions'};
	case '/history':
		return {kind: 'history'};
	case '/export':
		return {kind: 'export'};
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
		return null;
	}
}

function parseSlashCommand(line: string): Command {
	const fields = line.split(/\s+/);
	const head = fields[0] ?? '';
	const simpleCommand = parseSimpleSlashCommand(head);
	if (simpleCommand !== null) {
		return simpleCommand;
	}

	switch (head) {
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
	case '/search': {
		const keyword = trimPrefix(line, '/search');
		if (keyword === '') {
			return {kind: 'invalid', message: 'usage: /search {keyword}'};
		}
		return {kind: 'search', keyword};
	}
	case '/cron': {
		if (fields.length === 1 || fields[1] === 'list') {
			return {kind: 'cron_list'};
		}
		const sub = fields[1] ?? '';
		if (sub === 'add') {
			if (fields.length < 4) {
				return {kind: 'invalid', message: 'usage: /cron add {schedule} {prompt}'};
			}
			const schedule = (fields[2] ?? '').trim();
			const prompt = trimPrefix(line, `/cron add ${schedule}`);
			if (schedule === '' || prompt === '') {
				return {kind: 'invalid', message: 'usage: /cron add {schedule} {prompt}'};
			}
			return {kind: 'cron_add', schedule, prompt};
		}
		if (sub === 'run') {
			if (fields.length < 3 || (fields[2] ?? '').trim() === '') {
				return {kind: 'invalid', message: 'usage: /cron run {job_id}'};
			}
			return {kind: 'cron_run', jobID: (fields[2] ?? '').trim()};
		}
		if (sub === 'delete') {
			if (fields.length < 3 || (fields[2] ?? '').trim() === '') {
				return {kind: 'invalid', message: 'usage: /cron delete {job_id}'};
			}
			return {kind: 'cron_delete', jobID: (fields[2] ?? '').trim()};
		}
		if (sub === 'enable') {
			if (fields.length < 3 || (fields[2] ?? '').trim() === '') {
				return {kind: 'invalid', message: 'usage: /cron enable {job_id}'};
			}
			return {kind: 'cron_enable', jobID: (fields[2] ?? '').trim()};
		}
		if (sub === 'disable') {
			if (fields.length < 3 || (fields[2] ?? '').trim() === '') {
				return {kind: 'invalid', message: 'usage: /cron disable {job_id}'};
			}
			return {kind: 'cron_disable', jobID: (fields[2] ?? '').trim()};
		}
		return {kind: 'invalid', message: 'usage: /cron {list|add|run|delete|enable|disable}'};
	}
	case '/notify': {
		if (fields.length === 1 || fields[1] === 'list') {
			return {kind: 'notify_list'};
		}
		const sub = fields[1] ?? '';
		if (sub === 'filter') {
			const filter = (fields[2] ?? '').trim();
			if (filter !== 'all' && filter !== 'cron' && filter !== 'heartbeat' && filter !== 'error') {
				return {kind: 'invalid', message: 'usage: /notify filter {all|cron|heartbeat|error}'};
			}
			return {kind: 'notify_filter', filter};
		}
		if (sub === 'open') {
			const rawIndex = (fields[2] ?? '').trim();
			const index = Number(rawIndex);
			if (rawIndex === '' || !Number.isInteger(index) || index < 1) {
				return {kind: 'invalid', message: 'usage: /notify open {index}'};
			}
			return {kind: 'notify_open', index};
		}
		if (sub === 'clear') {
			return {kind: 'notify_clear'};
		}
		return {kind: 'invalid', message: 'usage: /notify {list|filter|open|clear}'};
	}
	default:
		return {kind: 'invalid', message: `unknown command: ${head}`};
	}
}

export function parseInputCommand(raw: string): Command {
	const line = normalizeCommandPrefix(raw.trim());
	if (line === '') {
		return {kind: 'noop'};
	}
	if (!line.startsWith('/')) {
		return {kind: 'chat', message: line};
	}
	return parseSlashCommand(line);
}
