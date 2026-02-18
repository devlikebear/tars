import fs from 'node:fs';

type CliConfig = {
	serverUrl?: string;
	casedServerUrl?: string;
	sessionId?: string;
	verbose?: boolean;
};

function parseBool(raw: string): boolean | undefined {
	const v = raw.trim().toLowerCase();
	if (v === 'true' || v === '1' || v === 'yes' || v === 'on') {
		return true;
	}
	if (v === 'false' || v === '0' || v === 'no' || v === 'off') {
		return false;
	}
	return undefined;
}

function unquote(value: string): string {
	const trimmed = value.trim();
	if ((trimmed.startsWith('"') && trimmed.endsWith('"')) || (trimmed.startsWith("'") && trimmed.endsWith("'"))) {
		return trimmed.slice(1, -1);
	}
	return trimmed;
}

export function loadCliConfig(path: string): CliConfig {
	const text = fs.readFileSync(path, 'utf-8');
	const out: CliConfig = {};
	const lines = text.split(/\r?\n/);
	for (const line of lines) {
		const trimmed = line.trim();
		if (trimmed === '' || trimmed.startsWith('#')) {
			continue;
		}
		const idx = trimmed.indexOf(':');
		if (idx <= 0) {
			continue;
		}
		const key = trimmed.slice(0, idx).trim();
		const value = unquote(trimmed.slice(idx + 1));
		switch (key) {
		case 'server_url':
			out.serverUrl = value;
			break;
		case 'cased_server_url':
			out.casedServerUrl = value;
			break;
		case 'session_id':
			out.sessionId = value;
			break;
		case 'verbose': {
			const parsed = parseBool(value);
			if (typeof parsed === 'boolean') {
				out.verbose = parsed;
			}
			break;
		}
		}
	}
	return out;
}
