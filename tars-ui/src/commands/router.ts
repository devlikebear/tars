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
	| {kind: 'skills'}
	| {kind: 'plugins'}
	| {kind: 'mcp'}
	| {kind: 'agents'; detail?: boolean}
	| {kind: 'reload'}
	| {kind: 'runs'}
	| {kind: 'spawn'; message: string; agent?: string; title?: string; sessionID?: string; wait?: boolean}
	| {kind: 'run'; runID: string}
	| {kind: 'cancel_run'; runID: string}
	| {kind: 'gateway'; action: 'status' | 'reload' | 'restart'}
	| {kind: 'channels'}
	| {kind: 'cron_list'}
	| {kind: 'cron_add'; schedule: string; prompt: string}
	| {kind: 'cron_get'; jobID: string}
	| {kind: 'cron_runs'; jobID: string; limit: number}
	| {kind: 'cron_run'; jobID: string}
	| {kind: 'cron_delete'; jobID: string}
	| {kind: 'cron_enable'; jobID: string}
	| {kind: 'cron_disable'; jobID: string}
	| {kind: 'notify_list'}
	| {kind: 'notify_filter'; filter: 'all' | 'cron' | 'heartbeat' | 'error'}
	| {kind: 'notify_open'; index: number}
	| {kind: 'notify_clear'}
	| {kind: 'skill_invoke'; skillName: string; message: string}
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
	case '/skills':
		return {kind: 'skills'};
	case '/plugins':
		return {kind: 'plugins'};
	case '/mcp':
		return {kind: 'mcp'};
	case '/reload':
		return {kind: 'reload'};
	case '/runs':
		return {kind: 'runs'};
	case '/channels':
		return {kind: 'channels'};
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
	case '/agents': {
		if (fields.length === 1) {
			return {kind: 'agents'};
		}
		if (fields.length === 2 && ((fields[1] ?? '').trim() === '--detail' || (fields[1] ?? '').trim() === '-d')) {
			return {kind: 'agents', detail: true};
		}
		return {kind: 'invalid', message: 'usage: /agents [--detail]'};
	}
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
		if (sub === 'get') {
			if (fields.length < 3 || (fields[2] ?? '').trim() === '') {
				return {kind: 'invalid', message: 'usage: /cron get {job_id}'};
			}
			return {kind: 'cron_get', jobID: (fields[2] ?? '').trim()};
		}
		if (sub === 'runs') {
			if (fields.length < 3 || (fields[2] ?? '').trim() === '') {
				return {kind: 'invalid', message: 'usage: /cron runs {job_id} [limit]'};
			}
			const jobID = (fields[2] ?? '').trim();
			const rawLimit = (fields[3] ?? '').trim();
			let limit = 20;
			if (rawLimit !== '') {
				const parsed = Number(rawLimit);
				if (!Number.isInteger(parsed) || parsed <= 0) {
					return {kind: 'invalid', message: 'usage: /cron runs {job_id} [limit]'};
				}
				limit = parsed;
			}
			return {kind: 'cron_runs', jobID, limit};
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
		return {kind: 'invalid', message: 'usage: /cron {list|get|runs|add|run|delete|enable|disable}'};
	}
	case '/run': {
		if (fields.length < 2 || (fields[1] ?? '').trim() === '') {
			return {kind: 'invalid', message: 'usage: /run {run_id}'};
		}
		return {kind: 'run', runID: (fields[1] ?? '').trim()};
	}
	case '/spawn': {
		const usage = 'usage: /spawn [--agent {name}] [--title {title}] [--session {id}] [--wait] {message}';
		let agent = '';
		let title = '';
		let sessionID = '';
		let wait = false;
		let idx = 1;
		for (; idx < fields.length; idx++) {
			const token = (fields[idx] ?? '').trim();
			if (token === '') {
				continue;
			}
			if (token === '--') {
				idx++;
				break;
			}
			if (token === '--wait') {
				wait = true;
				continue;
			}
			if (token === '--agent' || token === '--title' || token === '--session') {
				if (idx+1 >= fields.length) {
					return {kind: 'invalid', message: usage};
				}
				const value = (fields[idx+1] ?? '').trim();
				if (value === '') {
					return {kind: 'invalid', message: usage};
				}
				if (token === '--agent') {
					agent = value;
				} else if (token === '--title') {
					title = value;
				} else {
					sessionID = value;
				}
				idx++;
				continue;
			}
			if (token.startsWith('--agent=')) {
				agent = token.slice('--agent='.length).trim();
				if (agent === '') {
					return {kind: 'invalid', message: usage};
				}
				continue;
			}
			if (token.startsWith('--title=')) {
				title = token.slice('--title='.length).trim();
				if (title === '') {
					return {kind: 'invalid', message: usage};
				}
				continue;
			}
			if (token.startsWith('--session=')) {
				sessionID = token.slice('--session='.length).trim();
				if (sessionID === '') {
					return {kind: 'invalid', message: usage};
				}
				continue;
			}
			if (token.startsWith('--')) {
				return {kind: 'invalid', message: usage};
			}
			break;
		}
		const message = fields.slice(idx).join(' ').trim();
		if (message === '') {
			return {kind: 'invalid', message: usage};
		}
		const out: {kind: 'spawn'; message: string; agent?: string; title?: string; sessionID?: string; wait?: boolean} = {
			kind: 'spawn',
			message,
		};
		if (agent !== '') {
			out.agent = agent;
		}
		if (title !== '') {
			out.title = title;
		}
		if (sessionID !== '') {
			out.sessionID = sessionID;
		}
		if (wait) {
			out.wait = true;
		}
		return out;
	}
	case '/cancel-run': {
		if (fields.length < 2 || (fields[1] ?? '').trim() === '') {
			return {kind: 'invalid', message: 'usage: /cancel-run {run_id}'};
		}
		return {kind: 'cancel_run', runID: (fields[1] ?? '').trim()};
	}
	case '/gateway': {
		const sub = (fields[1] ?? 'status').trim();
		if (sub !== 'status' && sub !== 'reload' && sub !== 'restart') {
			return {kind: 'invalid', message: 'usage: /gateway {status|reload|restart}'};
		}
		return {kind: 'gateway', action: sub};
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
		if (head.startsWith('/')) {
			const skillName = head.slice(1).trim();
			if (/^[a-zA-Z0-9_.-]+$/.test(skillName)) {
				return {kind: 'skill_invoke', skillName, message: line};
			}
		}
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
