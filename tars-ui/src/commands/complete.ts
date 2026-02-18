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
	'/agents',
	'/reload',
	'/runs',
	'/spawn',
	'/run',
	'/cancel-run',
	'/gateway',
	'/sentinel',
	'/channels',
	'/cron',
	'/notify',
	'/quit',
];

const cronSubs = ['list', 'add', 'get', 'runs', 'run', 'delete', 'enable', 'disable'];
const notifySubs = ['list', 'filter', 'open', 'clear'];
const gatewaySubs = ['status', 'reload', 'restart'];
const sentinelSubs = ['status', 'restart', 'pause', 'resume', 'events'];
const spawnOptions = ['--agent', '--title', '--session', '--wait'];

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
	} else if (head === '/gateway') {
		completedSub = completeToken(subPrefix, gatewaySubs);
	} else if (head === '/sentinel') {
		completedSub = completeToken(subPrefix, sentinelSubs);
	} else if (head === '/spawn') {
		if (!line.endsWith(' ')) {
			const tokens = rest.split(/\s+/).filter((token) => token.trim() !== '');
			const lastIdx = tokens.length - 1;
			const last = tokens[lastIdx] ?? '';
			if (last.startsWith('--')) {
				const completed = completeToken(last, spawnOptions);
				if (completed !== null) {
					tokens[lastIdx] = completed.value;
					const suffix = completed.unique ? ' ' : '';
					return `/spawn ${tokens.join(' ')}${suffix}`;
				}
			}
		}
	}
	if (completedSub === null) {
		return line;
	}
	return completedSub.unique ? `/${head.slice(1)} ${completedSub.value} ` : `/${head.slice(1)} ${completedSub.value}`;
}
