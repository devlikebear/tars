import test from 'node:test';
import assert from 'node:assert/strict';
import {CommandAPIs, CommandExecutorContext, executeInputCommand} from './commandExecutor.js';
import {SessionSummary} from '../types.js';

function createDefaultAPIs(): CommandAPIs {
	const unexpected = async (): Promise<never> => {
		throw new Error('unexpected api call');
	};
	return {
		listSessions: unexpected,
		createSession: unexpected,
		getSession: unexpected,
		getHistory: unexpected,
		exportSession: unexpected,
		searchSessions: unexpected,
		getStatus: unexpected,
		runCompact: unexpected,
		runHeartbeatOnce: unexpected,
		listCronJobs: unexpected,
		createCronJob: unexpected,
		updateCronJob: unexpected,
		runCronJob: unexpected,
		getCronJob: unexpected,
		listCronRuns: unexpected,
		deleteCronJob: unexpected,
		listSkills: unexpected,
		listPlugins: unexpected,
		listMCPServers: unexpected,
		listMCPTools: unexpected,
		reloadExtensions: unexpected,
			listAgentRuns: unexpected,
			listAgents: unexpected,
			spawnAgentRun: unexpected,
			getAgentRun: unexpected,
		cancelAgentRun: unexpected,
		getGatewayStatus: unexpected,
		reloadGateway: unexpected,
		restartGateway: unexpected,
	};
}

function createContext(raw: string, sessionID = ''): {
	ctx: CommandExecutorContext;
	errors: string[];
	messages: string[];
	tables: Array<{headers: string[]; rows: string[][]}>;
	resumeCandidates: SessionSummary[] | null;
	resumeIndex: number;
	activeSessionID: string;
	notificationFilter: string;
	notifications: Array<{id: number; category: string; severity: string; title: string; message: string; timestamp: string}>;
	cronRunsPreview: string[];
	markNotificationsSeenCalls: number;
	exited: boolean;
} {
	const errors: string[] = [];
	const messages: string[] = [];
	const tables: Array<{headers: string[]; rows: string[][]}> = [];

	let resumeCandidates: SessionSummary[] | null = null;
	let resumeIndex = -1;
	let activeSessionID = sessionID;
	let notificationFilter = 'all';
	let notifications: Array<{id: number; category: string; severity: string; title: string; message: string; timestamp: string}> = [];
	let cronRunsPreview: string[] = [];
	let markNotificationsSeenCalls = 0;
	let exited = false;

	const ctx: CommandExecutorContext = {
		raw,
		serverUrl: 'http://127.0.0.1:8080',
		sessionID,
		pushSystemMessage: (text) => messages.push(text),
		pushSystemTable: (headers, rows) => tables.push({headers, rows}),
		pushErrorMessage: (text) => errors.push(text),
		setSessionID: (id) => {
			activeSessionID = id;
		},
		setResumeCandidates: (sessions) => {
			resumeCandidates = sessions;
		},
		setResumeIndex: (index) => {
			resumeIndex = index;
		},
		setNotificationFilter: (next) => {
			notificationFilter = next;
		},
		getNotificationFilter: () => notificationFilter as 'all' | 'cron' | 'heartbeat' | 'error',
		getNotifications: () => notifications,
		clearNotifications: () => {
			notifications = [];
		},
		markNotificationsSeen: () => {
			markNotificationsSeenCalls++;
		},
		setCronRunsPreview: (lines) => {
			cronRunsPreview = [...lines];
		},
		exit: () => {
			exited = true;
		},
	};

	return {
		ctx,
		errors,
		messages,
		tables,
		get resumeCandidates() {
			return resumeCandidates;
		},
		get resumeIndex() {
			return resumeIndex;
		},
		get activeSessionID() {
			return activeSessionID;
		},
		get notificationFilter() {
			return notificationFilter;
		},
		set notifications(value) {
			notifications = value;
		},
		get notifications() {
			return notifications;
		},
		get cronRunsPreview() {
			return cronRunsPreview;
		},
		get markNotificationsSeenCalls() {
			return markNotificationsSeenCalls;
		},
		get exited() {
			return exited;
		},
	};
}

test('executeInputCommand returns invalid command usage via error message', async () => {
	const state = createContext('/search');
	await executeInputCommand(state.ctx, createDefaultAPIs());

	assert.deepEqual(state.errors, ['usage: /search {keyword}']);
	assert.deepEqual(state.messages, []);
	assert.deepEqual(state.tables, []);
});

test('executeInputCommand handles /quit by exiting', async () => {
	const state = createContext('/quit');
	await executeInputCommand(state.ctx, createDefaultAPIs());

	assert.equal(state.exited, true);
});

test('executeInputCommand handles /resume without sessions list', async () => {
	const apis: CommandAPIs = {
		...createDefaultAPIs(),
		listSessions: async () => [],
	};
	const state = createContext('/resume');
	await executeInputCommand(state.ctx, apis);

	assert.deepEqual(state.messages, ['(no sessions)']);
	assert.equal(state.resumeCandidates, null);
	assert.equal(state.resumeIndex, -1);
});

test('executeInputCommand handles /resume with selection mode', async () => {
	const sessions = [
		{id: 'sess-1', title: 'first'},
		{id: 'sess-2', title: 'second'},
	];
	const apis: CommandAPIs = {
		...createDefaultAPIs(),
		listSessions: async () => sessions,
	};
	const state = createContext('/resume');
	await executeInputCommand(state.ctx, apis);

	assert.deepEqual(state.messages, ['Use ↑/↓ and Enter to select a session.']);
	assert.deepEqual(state.resumeCandidates, sessions);
	assert.equal(state.resumeIndex, 0);
});

test('executeInputCommand blocks history when session is missing', async () => {
	const state = createContext('/history', '');
	await executeInputCommand(state.ctx, createDefaultAPIs());

	assert.deepEqual(state.errors, ['no active session. use /new or /resume {session_id}']);
});

test('executeInputCommand handles /new and updates active session', async () => {
	const apis: CommandAPIs = {
		...createDefaultAPIs(),
		createSession: async () => ({id: 'sess-10', title: 'chat'}),
	};
	const state = createContext('/new hello');
	await executeInputCommand(state.ctx, apis);

	assert.equal(state.activeSessionID, 'sess-10');
	assert.deepEqual(state.messages, ['active session: sess-10']);
});

test('executeInputCommand clears resume selection when creating a new session', async () => {
	const apis: CommandAPIs = {
		...createDefaultAPIs(),
		createSession: async () => ({id: 'sess-11', title: 'new chat'}),
	};
	const state = createContext('/new');
	state.ctx.setResumeCandidates([
		{id: 'sess-old-1', title: 'old one'},
		{id: 'sess-old-2', title: 'old two'},
	]);
	state.ctx.setResumeIndex(1);

	await executeInputCommand(state.ctx, apis);

	assert.equal(state.activeSessionID, 'sess-11');
	assert.equal(state.resumeCandidates, null);
	assert.equal(state.resumeIndex, 0);
	assert.deepEqual(state.messages, ['active session: sess-11']);
});

test('executeInputCommand handles /cron list', async () => {
	const apis: CommandAPIs = {
		...createDefaultAPIs(),
		listCronJobs: async () => [{id: 'job_1', name: 'morning', prompt: 'ping', schedule: 'every:1h', enabled: true, delete_after_run: false}],
	};
	const state = createContext('/cron list');
	await executeInputCommand(state.ctx, apis);

	assert.equal(state.tables.length, 1);
	assert.deepEqual(state.tables[0]?.headers, ['ID', 'NAME', 'SCHEDULE', 'ENABLED']);
	assert.deepEqual(state.tables[0]?.rows[0], ['job_1', 'morning', 'every:1h', 'yes']);
});

test('executeInputCommand handles /skills /plugins /mcp', async () => {
	const apis: CommandAPIs = {
		...createDefaultAPIs(),
		listSkills: async () => [{name: 'deploy', description: 'deploy skill', user_invocable: true, source: 'workspace', file_path: 'skills/deploy/SKILL.md', runtime_path: '_shared/skills_runtime/deploy/SKILL.md'}],
		listPlugins: async () => [{id: 'ops', name: 'Ops', source: 'workspace', root_dir: '/tmp/plugins/ops', manifest_path: '/tmp/plugins/ops/tarsncase.plugin.json'}],
		listMCPServers: async () => [{name: 'filesystem', command: 'npx', connected: true, tool_count: 1}],
		listMCPTools: async () => [{server: 'filesystem', name: 'read_file', description: 'read'}],
	};

	const skillsState = createContext('/skills');
	await executeInputCommand(skillsState.ctx, apis);
	assert.equal(skillsState.tables.length, 1);
	assert.deepEqual(skillsState.tables[0]?.headers, ['NAME', 'INVOKE', 'SOURCE', 'DESCRIPTION', 'RUNTIME_PATH']);

	const pluginsState = createContext('/plugins');
	await executeInputCommand(pluginsState.ctx, apis);
	assert.equal(pluginsState.tables.length, 1);
	assert.deepEqual(pluginsState.tables[0]?.headers, ['ID', 'NAME', 'SOURCE', 'VERSION', 'ROOT_DIR']);

	const mcpState = createContext('/mcp');
	await executeInputCommand(mcpState.ctx, apis);
	assert.equal(mcpState.tables.length, 2);
	assert.deepEqual(mcpState.tables[0]?.headers, ['NAME', 'CONNECTED', 'TOOLS', 'COMMAND', 'ERROR']);
	assert.deepEqual(mcpState.tables[1]?.headers, ['SERVER', 'NAME', 'DESCRIPTION']);
});

test('executeInputCommand handles /reload', async () => {
	const apis: CommandAPIs = {
		...createDefaultAPIs(),
		reloadExtensions: async () => ({reloaded: true, version: 9, skills: 3, plugins: 2, mcp_count: 4, gateway_refreshed: true, gateway_agents: 2}),
	};
	const state = createContext('/reload');
	await executeInputCommand(state.ctx, apis);
	assert.deepEqual(state.messages, ['extensions reloaded: version=9 skills=3 plugins=2 mcp=4 gateway_refresh=yes gateway_agents=2']);
});

test('executeInputCommand handles /agents /spawn /runs /run /cancel-run /gateway /channels', async () => {
	const spawnInputs: Array<{session_id?: string; title?: string; message: string; agent?: string}> = [];
	let getRunCalls = 0;
	const apis: CommandAPIs = {
		...createDefaultAPIs(),
		listAgents: async () => [{name: 'default', description: 'Default in-process agent loop', enabled: true, kind: 'prompt', default: true, source: 'in-process', entry: 'agent-loop'}],
		spawnAgentRun: async (_serverUrl, input) => {
			spawnInputs.push(input);
			return {run_id: 'run_0', session_id: input.session_id ?? 'sess_0', status: 'accepted', accepted: true, agent: input.agent};
		},
		listAgentRuns: async () => [{run_id: 'run_1', session_id: 'sess_1', status: 'running', accepted: true}],
		getAgentRun: async (_serverUrl, runID) => {
			getRunCalls++;
			if (runID === 'run_0') {
				return {run_id: 'run_0', session_id: 'sess_0', status: 'completed', accepted: true, response: 'spawn done'};
			}
			return {run_id: 'run_1', session_id: 'sess_1', status: 'completed', accepted: true, response: 'ok'};
		},
		cancelAgentRun: async () => ({run_id: 'run_1', session_id: 'sess_1', status: 'canceled', accepted: true}),
		getGatewayStatus: async () => ({
			enabled: true,
			version: 2,
			runs_total: 3,
			runs_active: 1,
			agents_count: 2,
			agents_watch_enabled: true,
			agents_reload_version: 5,
			agents_last_reload_at: '2026-02-17T12:00:00Z',
			channels_local_enabled: true,
			channels_webhook_enabled: false,
			channels_telegram_enabled: true,
		}),
		reloadGateway: async () => ({
			enabled: true,
			version: 3,
			runs_total: 3,
			runs_active: 1,
			agents_count: 2,
			agents_watch_enabled: true,
			agents_reload_version: 6,
			agents_last_reload_at: '2026-02-17T12:01:00Z',
			channels_local_enabled: true,
			channels_webhook_enabled: false,
			channels_telegram_enabled: true,
		}),
		restartGateway: async () => ({
			enabled: true,
			version: 4,
			runs_total: 2,
			runs_active: 0,
			agents_count: 2,
			agents_watch_enabled: true,
			agents_reload_version: 6,
			agents_last_reload_at: '2026-02-17T12:01:00Z',
			channels_local_enabled: true,
			channels_webhook_enabled: false,
			channels_telegram_enabled: true,
		}),
	};

	const agentsState = createContext('/agents');
	await executeInputCommand(agentsState.ctx, apis);
	assert.equal(agentsState.tables.length, 1);
	assert.deepEqual(agentsState.tables[0]?.headers, ['NAME', 'DEFAULT', 'ENABLED', 'KIND', 'DESCRIPTION']);

	const agentsDetailState = createContext('/agents --detail');
	await executeInputCommand(agentsDetailState.ctx, apis);
	assert.equal(agentsDetailState.tables.length, 1);
	assert.deepEqual(agentsDetailState.tables[0]?.headers, ['NAME', 'DEFAULT', 'ENABLED', 'KIND', 'SOURCE', 'ENTRY', 'DESCRIPTION']);

	const spawnState = createContext('/spawn summarize today');
	await executeInputCommand(spawnState.ctx, apis);
	assert.deepEqual(spawnState.messages, ['run accepted: run_0 status=accepted']);
	assert.deepEqual(spawnInputs[0], {message: 'summarize today'});

	const spawnWithSessionState = createContext('/spawn --agent worker --title nightly --session sess_x summarize today', 'sess_current');
	await executeInputCommand(spawnWithSessionState.ctx, apis);
	assert.deepEqual(spawnWithSessionState.messages, ['run accepted: run_0 status=accepted']);
	assert.deepEqual(spawnInputs[1], {message: 'summarize today', agent: 'worker', title: 'nightly', session_id: 'sess_x'});

	const spawnWaitState = createContext('/spawn --wait summarize and finish');
	await executeInputCommand(spawnWaitState.ctx, apis);
	assert.deepEqual(spawnWaitState.messages, ['run completed: run_0 status=completed']);
	assert.equal(getRunCalls > 0, true);

	const runsState = createContext('/runs');
	await executeInputCommand(runsState.ctx, apis);
	assert.equal(runsState.tables.length, 1);
	assert.deepEqual(runsState.tables[0]?.headers, ['RUN_ID', 'SESSION', 'STATUS', 'AGENT', 'DETAIL']);

	const runState = createContext('/run run_1');
	await executeInputCommand(runState.ctx, apis);
	assert.equal(runState.tables.length, 1);
	assert.deepEqual(runState.tables[0]?.headers, ['FIELD', 'VALUE']);

	const cancelState = createContext('/cancel-run run_1');
	await executeInputCommand(cancelState.ctx, apis);
	assert.deepEqual(cancelState.messages, ['run canceled: run_1 status=canceled']);

	const gatewayState = createContext('/gateway');
	await executeInputCommand(gatewayState.ctx, apis);
	assert.equal(gatewayState.tables.length, 1);
	assert.deepEqual(gatewayState.tables[0]?.headers, ['FIELD', 'VALUE']);
	assert.equal(gatewayState.tables[0]?.rows.some((row) => row[0] === 'agents_count'), true);

	const channelsState = createContext('/channels');
	await executeInputCommand(channelsState.ctx, apis);
	assert.equal(channelsState.tables.length, 1);
	assert.deepEqual(channelsState.tables[0]?.headers, ['FIELD', 'VALUE']);
});

test('executeInputCommand handles /cron add and /cron run /cron delete', async () => {
	const apis: CommandAPIs = {
		...createDefaultAPIs(),
		createCronJob: async () => ({id: 'job_2', name: 'nightly', prompt: 'check mail', schedule: 'every:30m', enabled: true, delete_after_run: false}),
		updateCronJob: async () => ({id: 'job_2', name: 'nightly', prompt: 'check mail', schedule: 'every:30m', enabled: true, delete_after_run: false}),
		runCronJob: async () => 'ran',
		getCronJob: async () => ({id: 'job_2', name: 'nightly', prompt: 'check mail', schedule: 'every:30m', enabled: true, delete_after_run: false}),
		listCronRuns: async () => [],
		deleteCronJob: async () => undefined,
	};
	const addState = createContext('/cron add every:30m check mail');
	await executeInputCommand(addState.ctx, apis);
	assert.deepEqual(addState.messages, ['cron job created: job_2']);

	const runState = createContext('/cron run job_2');
	await executeInputCommand(runState.ctx, apis);
	assert.deepEqual(runState.messages, ['ran']);

	const delState = createContext('/cron delete job_2');
	await executeInputCommand(delState.ctx, apis);
	assert.deepEqual(delState.messages, ['cron job deleted: job_2']);

	const enableState = createContext('/cron enable job_2');
	await executeInputCommand(enableState.ctx, apis);
	assert.deepEqual(enableState.messages, ['cron job enabled: job_2']);

	const disableState = createContext('/cron disable job_2');
	await executeInputCommand(disableState.ctx, apis);
	assert.deepEqual(disableState.messages, ['cron job disabled: job_2']);
});

test('executeInputCommand handles /cron get and /cron runs', async () => {
	const apis: CommandAPIs = {
		...createDefaultAPIs(),
		getCronJob: async () => ({
			id: 'job_9',
			name: 'nightly',
			prompt: 'check logs',
			schedule: 'every:1h',
			enabled: true,
			delete_after_run: true,
			last_run_at: '2026-02-16T11:00:00Z',
			last_run_error: '',
		}),
		listCronRuns: async () => ([
			{job_id: 'job_9', ran_at: '2026-02-16T11:00:00Z', response: 'done', error: ''},
			{job_id: 'job_9', ran_at: '2026-02-16T10:00:00Z', response: '', error: 'timeout'},
		]),
	};

	const getState = createContext('/cron get job_9');
	await executeInputCommand(getState.ctx, apis);
	assert.equal(getState.tables.length, 1);
	assert.deepEqual(getState.tables[0]?.headers, ['FIELD', 'VALUE']);
	assert.equal(getState.tables[0]?.rows[0]?.[0], 'id');
	assert.equal(getState.tables[0]?.rows[0]?.[1], 'job_9');

	const runsState = createContext('/cron runs job_9 2');
	await executeInputCommand(runsState.ctx, apis);
	assert.equal(runsState.tables.length, 1);
	assert.deepEqual(runsState.tables[0]?.headers, ['TIME', 'STATUS', 'DETAIL']);
	assert.equal(runsState.tables[0]?.rows.length, 2);
	assert.equal(runsState.tables[0]?.rows[0]?.[1], 'ok');
	assert.equal(runsState.tables[0]?.rows[1]?.[1], 'error');
	assert.equal(runsState.cronRunsPreview[0], 'job=job_9');
	assert.equal(runsState.cronRunsPreview.length, 3);
	assert.equal(runsState.cronRunsPreview[1]?.includes('| ok |'), true);
	assert.equal(runsState.cronRunsPreview[2]?.includes('| error |'), true);
});

test('executeInputCommand handles /notify list/filter/open/clear', async () => {
	const state = createContext('/notify list');
	state.notifications = [
		{id: 1, category: 'cron', severity: 'info', title: 'Cron done', message: 'job complete', timestamp: '2026-02-16T10:00:00Z'},
		{id: 2, category: 'heartbeat', severity: 'error', title: 'Heartbeat failed', message: 'auth failed', timestamp: '2026-02-16T10:01:00Z'},
	];

	await executeInputCommand(state.ctx, createDefaultAPIs());
	assert.equal(state.tables.length, 1);
	assert.deepEqual(state.tables[0]?.headers, ['#', 'CATEGORY', 'SEVERITY', 'TITLE', 'TIME']);
	assert.equal(state.markNotificationsSeenCalls, 1);

	const filterState = createContext('/notify filter cron');
	await executeInputCommand(filterState.ctx, createDefaultAPIs());
	assert.equal(filterState.notificationFilter, 'cron');
	assert.deepEqual(filterState.messages, ['notification filter: cron']);

	const openState = createContext('/notify open 2');
	openState.notifications = state.notifications;
	await executeInputCommand(openState.ctx, createDefaultAPIs());
	assert.equal(openState.messages.length, 1);
	assert.equal(openState.messages[0]?.includes('Heartbeat failed'), true);
	assert.equal(openState.markNotificationsSeenCalls, 1);

	const clearState = createContext('/notify clear');
	clearState.notifications = state.notifications;
	await executeInputCommand(clearState.ctx, createDefaultAPIs());
	assert.equal(clearState.notifications.length, 0);
	assert.deepEqual(clearState.messages, ['notifications cleared']);
	assert.equal(clearState.markNotificationsSeenCalls, 1);
});

test('executeInputCommand notify filter error uses severity', async () => {
	const state = createContext('/notify filter error');
	state.notifications = [
		{id: 1, category: 'cron', severity: 'info', title: 'Cron done', message: 'ok', timestamp: '2026-02-16T10:00:00Z'},
		{id: 2, category: 'heartbeat', severity: 'error', title: 'Heartbeat failed', message: 'down', timestamp: '2026-02-16T10:01:00Z'},
	];
	await executeInputCommand(state.ctx, createDefaultAPIs());
	assert.equal(state.notificationFilter, 'error');

	state.ctx.raw = '/notify list';
	await executeInputCommand(state.ctx, createDefaultAPIs());
	assert.equal(state.tables.length, 1);
	assert.equal(state.tables[0]?.rows.length, 1);
	assert.equal(state.tables[0]?.rows[0]?.[2], 'error');
});
