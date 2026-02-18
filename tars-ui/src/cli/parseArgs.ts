import {loadCliConfig} from './config.js';

export type CliOptions = {
	serverUrl: string;
	casedServerUrl: string;
	sessionId: string;
	apiToken: string;
	casedApiToken: string;
	workspaceId: string;
	verbose: boolean;
};

function resolveConfigPath(argv: string[]): string {
	for (let i = 0; i < argv.length; i += 1) {
		const arg = argv[i] ?? '';
		if (arg === '--config' && argv[i + 1]) {
			return argv[i + 1]!.trim();
		}
		if (arg.startsWith('--config=')) {
			return arg.slice('--config='.length).trim();
		}
	}
	return '';
}

export function parseArgs(argv: string[]): CliOptions {
	let serverUrl = 'http://127.0.0.1:43180';
	let casedServerUrl = 'http://127.0.0.1:43181';
	let sessionId = '';
	let apiToken = '';
	let casedApiToken = '';
	let workspaceId = '';
	let verbose = false;
	const configPath = resolveConfigPath(argv);
	if (configPath !== '') {
		const fromFile = loadCliConfig(configPath);
		if ((fromFile.serverUrl ?? '').trim() !== '') {
			serverUrl = fromFile.serverUrl!.trim();
		}
		if ((fromFile.casedServerUrl ?? '').trim() !== '') {
			casedServerUrl = fromFile.casedServerUrl!.trim();
		}
		if ((fromFile.sessionId ?? '').trim() !== '') {
			sessionId = fromFile.sessionId!.trim();
		}
		if ((fromFile.apiToken ?? '').trim() !== '') {
			apiToken = fromFile.apiToken!.trim();
		}
		if ((fromFile.casedApiToken ?? '').trim() !== '') {
			casedApiToken = fromFile.casedApiToken!.trim();
		}
		if ((fromFile.workspaceId ?? '').trim() !== '') {
			workspaceId = fromFile.workspaceId!.trim();
		}
		if (typeof fromFile.verbose === 'boolean') {
			verbose = fromFile.verbose;
		}
	}

	for (let i = 0; i < argv.length; i += 1) {
		const arg = argv[i] ?? '';

		if (arg === '--verbose') {
			verbose = true;
			continue;
		}
		if (arg === '--server-url' && argv[i + 1]) {
			serverUrl = argv[i + 1]!;
			i += 1;
			continue;
		}
		if (arg.startsWith('--server-url=')) {
			serverUrl = arg.slice('--server-url='.length);
			continue;
		}
		if (arg === '--cased-url' && argv[i + 1]) {
			casedServerUrl = argv[i + 1]!;
			i += 1;
			continue;
		}
		if (arg.startsWith('--cased-url=')) {
			casedServerUrl = arg.slice('--cased-url='.length);
			continue;
		}
		if (arg === '--session' && argv[i + 1]) {
			sessionId = argv[i + 1]!;
			i += 1;
			continue;
		}
		if (arg.startsWith('--session=')) {
			sessionId = arg.slice('--session='.length);
			continue;
		}
		if (arg === '--api-token' && argv[i + 1]) {
			apiToken = argv[i + 1]!;
			i += 1;
			continue;
		}
		if (arg.startsWith('--api-token=')) {
			apiToken = arg.slice('--api-token='.length);
			continue;
		}
		if (arg === '--cased-api-token' && argv[i + 1]) {
			casedApiToken = argv[i + 1]!;
			i += 1;
			continue;
		}
		if (arg.startsWith('--cased-api-token=')) {
			casedApiToken = arg.slice('--cased-api-token='.length);
			continue;
		}
		if (arg === '--workspace-id' && argv[i + 1]) {
			workspaceId = argv[i + 1]!;
			i += 1;
			continue;
		}
		if (arg.startsWith('--workspace-id=')) {
			workspaceId = arg.slice('--workspace-id='.length);
		}
	}

	return {
		serverUrl: serverUrl.trim() || 'http://127.0.0.1:43180',
		casedServerUrl: casedServerUrl.trim() || 'http://127.0.0.1:43181',
		sessionId: sessionId.trim(),
		apiToken: apiToken.trim(),
		casedApiToken: casedApiToken.trim(),
		workspaceId: workspaceId.trim(),
		verbose,
	};
}
