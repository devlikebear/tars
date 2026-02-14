export type CliOptions = {
	serverUrl: string;
	sessionId: string;
	verbose: boolean;
};

export function parseArgs(argv: string[]): CliOptions {
	let serverUrl = 'http://127.0.0.1:8080';
	let sessionId = '';
	let verbose = false;

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
		if (arg === '--session' && argv[i + 1]) {
			sessionId = argv[i + 1]!;
			i += 1;
			continue;
		}
		if (arg.startsWith('--session=')) {
			sessionId = arg.slice('--session='.length);
		}
	}

	return {
		serverUrl: serverUrl.trim() || 'http://127.0.0.1:8080',
		sessionId: sessionId.trim(),
		verbose,
	};
}

