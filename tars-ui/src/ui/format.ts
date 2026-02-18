export function appendBounded(lines: string[], next: string, max: number): string[] {
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

export function commandHelpText(): string {
	return 'Commands: /help, /sessions, /new [title], /resume {id}, /history, /export, /search {keyword}, /status, /compact, /heartbeat, /skills, /plugins, /mcp, /agents [--detail], /reload, /runs, /spawn [--agent {name}] [--title {title}] [--session {id}] [--wait] {message}, /run {id}, /cancel-run {id}, /gateway {status|reload|restart}, /sentinel {status|restart|pause|resume|events [limit]}, /channels, /cron {list|get|runs|add|run|delete|enable|disable}, /notify {list|filter|open|clear}, /{skill_name}, /quit';
}

export function requireSessionOrError(currentSession: string): string | null {
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

export function renderTable(headers: string[], rows: string[][]): string[] {
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

export function truncate(value: string, max: number): string {
	const runes = Array.from(value);
	if (runes.length <= max) {
		return value;
	}
	if (max <= 3) {
		return runes.slice(0, max).join('');
	}
	return runes.slice(0, max - 3).join('') + '...';
}
