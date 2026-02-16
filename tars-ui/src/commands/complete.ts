const rootCommands = [
	'/help',
	'/sessions',
	'/new',
	'/resume',
	'/history',
	'/export',
	'/search',
	'/status',
	'/compact',
	'/heartbeat',
	'/skills',
	'/plugins',
	'/mcp',
	'/reload',
	'/cron',
	'/notify',
	'/quit',
];

const cronSubs = ['list', 'add', 'get', 'runs', 'run', 'delete', 'enable', 'disable'];
const notifySubs = ['list', 'filter', 'open', 'clear'];

function commonPrefix(values: string[]): string {
	if (values.length === 0) {
		return '';
	}
	let out = values[0] ?? '';
	for (const value of values.slice(1)) {
		let i = 0;
		for (; i < out.length && i < value.length; i++) {
			if (out[i] !== value[i]) {
				break;
			}
		}
		out = out.slice(0, i);
		if (out === '') {
			return '';
		}
	}
	return out;
}

type CompletionMatch = {
	value: string;
	unique: boolean;
};

function completeToken(prefix: string, candidates: string[]): CompletionMatch | null {
	const matches = candidates.filter((candidate) => candidate.startsWith(prefix));
	if (matches.length === 0) {
		return null;
	}
	if (candidates.includes(prefix)) {
		return {value: prefix, unique: true};
	}
	if (matches.length === 1) {
		return {value: matches[0] ?? prefix, unique: true};
	}
	const common = commonPrefix(matches);
	if (common.length > prefix.length) {
		return {value: common, unique: false};
	}
	return null;
}

export function completeCommandInput(line: string): string {
	if (!line.startsWith('/')) {
		return line;
	}
	const firstSpace = line.indexOf(' ');
	if (firstSpace === -1) {
		const completed = completeToken(line, rootCommands);
		if (completed === null) {
			return line;
		}
		if (completed.value === line) {
			return line;
		}
		return completed.unique ? `${completed.value} ` : completed.value;
	}

	const head = line.slice(0, firstSpace);
	const rest = line.slice(firstSpace + 1);
	const subPrefix = rest.trim();
	if (subPrefix === '' && !line.endsWith(' ')) {
		return line;
	}

	let completedSub: CompletionMatch | null = null;
	if (head === '/cron') {
		completedSub = completeToken(subPrefix, cronSubs);
	} else if (head === '/notify') {
		completedSub = completeToken(subPrefix, notifySubs);
	}
	if (completedSub === null) {
		return line;
	}
	return completedSub.unique ? `/${head.slice(1)} ${completedSub.value} ` : `/${head.slice(1)} ${completedSub.value}`;
}
