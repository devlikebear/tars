import {createSession, exportSession, getHistory, getSession, listSessions, searchSessions} from '../api/session.js';
import {createCronJob, deleteCronJob, getCronJob, listCronJobs, listCronRuns, runCronJob, updateCronJob} from '../api/cron.js';
import {listMCPServers, listMCPTools, listPlugins, listSkills, reloadExtensions} from '../api/extensions.js';
import {getStatus, runCompact, runHeartbeatOnce} from '../api/system.js';
import {cancelAgentRun, getAgentRun, getGatewayStatus, listAgentRuns, listAgents, reloadGateway, restartGateway, spawnAgentRun} from '../api/runtime.js';
import {getSentinelStatus, listSentinelEvents, pauseSentinel, restartSentinel, resumeSentinel} from '../api/sentinel.js';
import {parseInputCommand} from '../commands/router.js';
import {AgentDescriptor, AgentRunSummary, CronJob, CronRunRecord, GatewayStatus, MCPServerStatus, MCPToolInfo, NotificationItem, NotificationFilter, PluginDefinition, SentinelEvent, SentinelStatus, SessionHistoryItem, SessionSummary, SkillDefinition} from '../types.js';
import {commandHelpText, requireSessionOrError, truncate} from '../ui/format.js';

export type CommandAPIs = {
	listSessions: (serverUrl: string) => Promise<SessionSummary[]>;
	createSession: (serverUrl: string, title: string) => Promise<SessionSummary>;
	getSession: (serverUrl: string, sessionID: string) => Promise<SessionSummary>;
	getHistory: (serverUrl: string, sessionID: string) => Promise<SessionHistoryItem[]>;
	exportSession: (serverUrl: string, sessionID: string) => Promise<string>;
	searchSessions: (serverUrl: string, keyword: string) => Promise<SessionSummary[]>;
	getStatus: (serverUrl: string) => Promise<{workspace_dir: string; session_count: number}>;
	runCompact: (serverUrl: string, sessionID: string) => Promise<string>;
	runHeartbeatOnce: (serverUrl: string) => Promise<string>;
	listCronJobs: (serverUrl: string) => Promise<CronJob[]>;
	createCronJob: (serverUrl: string, input: {name?: string; prompt: string; schedule: string; enabled?: boolean; delete_after_run?: boolean}) => Promise<CronJob>;
	updateCronJob: (serverUrl: string, jobID: string, input: {name?: string; prompt?: string; schedule?: string; enabled?: boolean; delete_after_run?: boolean}) => Promise<CronJob>;
	runCronJob: (serverUrl: string, jobID: string) => Promise<string>;
	getCronJob: (serverUrl: string, jobID: string) => Promise<CronJob>;
	listCronRuns: (serverUrl: string, jobID: string, limit?: number) => Promise<CronRunRecord[]>;
	deleteCronJob: (serverUrl: string, jobID: string) => Promise<void>;
	listSkills: (serverUrl: string) => Promise<SkillDefinition[]>;
	listPlugins: (serverUrl: string) => Promise<PluginDefinition[]>;
	listMCPServers: (serverUrl: string) => Promise<MCPServerStatus[]>;
	listMCPTools: (serverUrl: string) => Promise<MCPToolInfo[]>;
	reloadExtensions: (serverUrl: string) => Promise<{reloaded: boolean; version?: number; skills?: number; plugins?: number; mcp_count?: number; gateway_refreshed?: boolean; gateway_agents?: number}>;
	listAgents: (serverUrl: string) => Promise<AgentDescriptor[]>;
	listAgentRuns: (serverUrl: string, limit?: number) => Promise<AgentRunSummary[]>;
	spawnAgentRun: (serverUrl: string, input: {session_id?: string; title?: string; message: string; agent?: string}) => Promise<AgentRunSummary>;
	getAgentRun: (serverUrl: string, runID: string) => Promise<AgentRunSummary>;
	cancelAgentRun: (serverUrl: string, runID: string) => Promise<AgentRunSummary>;
	getGatewayStatus: (serverUrl: string) => Promise<GatewayStatus>;
	reloadGateway: (serverUrl: string) => Promise<GatewayStatus>;
	restartGateway: (serverUrl: string) => Promise<GatewayStatus>;
	getSentinelStatus: (casedServerUrl: string) => Promise<SentinelStatus>;
	listSentinelEvents: (casedServerUrl: string, limit?: number) => Promise<SentinelEvent[]>;
	restartSentinel: (casedServerUrl: string) => Promise<SentinelStatus>;
	pauseSentinel: (casedServerUrl: string) => Promise<SentinelStatus>;
	resumeSentinel: (casedServerUrl: string) => Promise<SentinelStatus>;
};

const defaultAPIs: CommandAPIs = {
	listSessions,
	createSession,
	getSession,
	getHistory,
	exportSession,
	searchSessions,
	getStatus,
	runCompact,
	runHeartbeatOnce,
	listCronJobs,
	createCronJob,
	updateCronJob,
	runCronJob,
	getCronJob,
	listCronRuns,
	deleteCronJob,
	listSkills,
	listPlugins,
	listMCPServers,
	listMCPTools,
	reloadExtensions,
	listAgents,
	listAgentRuns,
	spawnAgentRun,
	getAgentRun,
	cancelAgentRun,
	getGatewayStatus,
	reloadGateway,
	restartGateway,
	getSentinelStatus,
	listSentinelEvents,
	restartSentinel,
	pauseSentinel,
	resumeSentinel,
};

export type CommandExecutorContext = {
	raw: string;
	serverUrl: string;
	casedServerUrl: string;
	sessionID: string;
	pushSystemMessage: (text: string) => void;
	pushSystemTable: (headers: string[], rows: string[][]) => void;
	pushErrorMessage: (text: string) => void;
	setSessionID: (sessionID: string) => void;
	setResumeCandidates: (sessions: SessionSummary[] | null) => void;
	setResumeIndex: (index: number) => void;
	setNotificationFilter: (next: NotificationFilter) => void;
	getNotificationFilter: () => NotificationFilter;
	getNotifications: () => NotificationItem[];
	clearNotifications: () => void;
	markNotificationsSeen: () => void;
	setCronRunsPreview: (lines: string[]) => void;
	exit: () => void;
};

function clearResumeSelection(ctx: CommandExecutorContext): void {
	ctx.setResumeCandidates(null);
	ctx.setResumeIndex(0);
}

function renderSessionRows(sessions: SessionSummary[]): string[][] {
	return sessions.map((session) => [session.id, truncate(session.title, 48)]);
}

function missingSessionError(sessionID: string): string | null {
	return requireSessionOrError(sessionID);
}

function renderCronRows(jobs: CronJob[]): string[][] {
	return jobs.map((job) => [job.id, truncate(job.name, 32), truncate(job.schedule, 18), job.enabled ? 'yes' : 'no']);
}

function renderCronDetailRows(job: CronJob): string[][] {
	const detailRows: string[][] = [
		['id', job.id],
		['name', job.name],
		['prompt', truncate(job.prompt, 80)],
		['schedule', job.schedule],
		['enabled', job.enabled ? 'yes' : 'no'],
		['delete_after_run', job.delete_after_run ? 'yes' : 'no'],
	];
	if ((job.session_target ?? '').trim() !== '') {
		detailRows.push(['session_target', job.session_target ?? '']);
	}
	if ((job.wake_mode ?? '').trim() !== '') {
		detailRows.push(['wake_mode', job.wake_mode ?? '']);
	}
	if ((job.delivery_mode ?? '').trim() !== '') {
		detailRows.push(['delivery_mode', job.delivery_mode ?? '']);
	}
	if ((job.last_run_at ?? '').trim() !== '') {
		detailRows.push(['last_run_at', job.last_run_at ?? '']);
	}
	if ((job.last_run_error ?? '').trim() !== '') {
		detailRows.push(['last_run_error', truncate(job.last_run_error ?? '', 80)]);
	}
	return detailRows;
}

function renderCronRunRows(runs: CronRunRecord[]): string[][] {
	return runs.map((run) => {
		const error = (run.error ?? '').trim();
		const response = (run.response ?? '').trim();
		return [
			run.ran_at,
			error === '' ? 'ok' : 'error',
			error === '' ? truncate(response, 72) : truncate(error, 72),
		];
	});
}

function renderCronRunPreviewLines(runs: CronRunRecord[]): string[] {
	if (runs.length === 0) {
		return ['(no runs)'];
	}
	return runs.map((run, idx) => {
		const error = (run.error ?? '').trim();
		const response = (run.response ?? '').trim();
		const status = error === '' ? 'ok' : 'error';
		const detail = error === '' ? truncate(response, 40) : truncate(error, 40);
		return `${idx + 1}. ${run.ran_at} | ${status} | ${detail}`;
	});
}

function renderSkillRows(skills: SkillDefinition[]): string[][] {
	return skills.map((item) => [
		item.name,
		item.user_invocable ? 'yes' : 'no',
		item.source,
		truncate(item.description, 48),
		truncate(item.runtime_path ?? '', 44),
	]);
}

function renderPluginRows(plugins: PluginDefinition[]): string[][] {
	return plugins.map((item) => [
		item.id,
		truncate(item.name ?? '-', 24),
		item.source,
		truncate(item.version ?? '-', 12),
		truncate(item.root_dir, 36),
	]);
}

function renderMCPServerRows(servers: MCPServerStatus[]): string[][] {
	return servers.map((item) => [
		item.name,
		item.connected ? 'yes' : 'no',
		String(item.tool_count),
		truncate(item.command, 28),
		truncate(item.error ?? '', 40),
	]);
}

function renderMCPToolRows(tools: MCPToolInfo[]): string[][] {
	return tools.map((item) => [item.server, item.name, truncate(item.description ?? '', 40)]);
}

function renderRunRows(runs: AgentRunSummary[]): string[][] {
	return runs.map((item) => [
		item.run_id,
		item.session_id ?? '-',
		item.status,
		item.agent ?? '-',
		truncate(item.response ?? item.error ?? '', 40),
	]);
}

function renderAgentRows(agents: AgentDescriptor[], detail: boolean): string[][] {
	return agents.map((item) => {
		const base = [
			item.name,
			item.default ? 'yes' : 'no',
			item.enabled ? 'yes' : 'no',
			truncate(item.kind ?? '-', 12),
		];
		if (!detail) {
			return [...base, truncate(item.description ?? '-', 48)];
		}
		const policyMode = truncate(item.policy_mode ?? 'full', 12);
		const allowCount = String(item.tools_allow_count ?? (Array.isArray(item.tools_allow) ? item.tools_allow.length : 0));
		return [
			...base,
			policyMode,
			allowCount,
			truncate(item.source ?? '-', 16),
			truncate(item.entry ?? '-', 28),
			truncate(item.description ?? '-', 48),
		];
	});
}

function isRunPending(status: string): boolean {
	const normalized = status.trim().toLowerCase();
	return normalized === 'accepted' || normalized === 'running';
}

function renderGatewayRows(status: GatewayStatus): string[][] {
	return [
		['enabled', status.enabled ? 'yes' : 'no'],
		['version', String(status.version ?? 0)],
		['runs_total', String(status.runs_total ?? 0)],
		['runs_active', String(status.runs_active ?? 0)],
		['agents_count', String(status.agents_count ?? 0)],
		['agents_watch', status.agents_watch_enabled ? 'yes' : 'no'],
		['agents_reload_version', String(status.agents_reload_version ?? 0)],
		['agents_last_reload_at', status.agents_last_reload_at ?? '-'],
		['channels_local', status.channels_local_enabled ? 'yes' : 'no'],
		['channels_webhook', status.channels_webhook_enabled ? 'yes' : 'no'],
		['channels_telegram', status.channels_telegram_enabled ? 'yes' : 'no'],
		['persistence', status.persistence_enabled ? 'yes' : 'no'],
		['runs_persistence', status.runs_persistence_enabled ? 'yes' : 'no'],
		['channels_persistence', status.channels_persistence_enabled ? 'yes' : 'no'],
		['restore_on_startup', status.restore_on_startup ? 'yes' : 'no'],
		['persistence_dir', status.persistence_dir ?? '-'],
		['runs_restored', String(status.runs_restored ?? 0)],
		['channels_restored', String(status.channels_restored ?? 0)],
		['last_persist_at', status.last_persist_at ?? '-'],
		['last_restore_at', status.last_restore_at ?? '-'],
		['last_restore_error', status.last_restore_error ?? '-'],
		['last_reload_at', status.last_reload_at ?? '-'],
		['last_restart_at', status.last_restart_at ?? '-'],
	];
}

function renderSentinelRows(status: SentinelStatus): string[][] {
	return [
		['enabled', status.enabled ? 'yes' : 'no'],
		['state', status.supervision_state ?? '-'],
		['target_command', status.target?.command ?? '-'],
		['target_args', truncate((status.target?.args ?? []).join(' '), 48)],
		['target_cwd', status.target?.cwd ?? '-'],
		['target_pid', status.target_pid != null ? String(status.target_pid) : '-'],
		['target_started_at', status.target_started_at ?? '-'],
		['target_last_exit_at', status.target_last_exit_at ?? '-'],
		['target_last_exit_code', status.target_last_exit_code != null ? String(status.target_last_exit_code) : '-'],
		['health_ok', status.health_ok ? 'yes' : 'no'],
		['health_last_ok_at', status.health_last_ok_at ?? '-'],
		['health_last_error', truncate(status.health_last_error ?? '', 72)],
		['restart_attempt', String(status.restart_attempt ?? 0)],
		['restart_max_attempts', String(status.restart_max_attempts ?? 0)],
		['cooldown_until', status.cooldown_until ?? '-'],
		['last_restart_at', status.last_restart_at ?? '-'],
		['event_count', String(status.event_count ?? 0)],
	];
}

function renderSentinelEventRows(events: SentinelEvent[]): string[][] {
	return events.map((item) => [
		String(item.id),
		item.time,
		item.level,
		item.type,
		truncate(item.message ?? '', 56),
	]);
}

function filterNotifications(items: NotificationItem[], filter: NotificationFilter): NotificationItem[] {
	if (filter === 'all') {
		return items;
	}
	if (filter === 'error') {
		return items.filter((item) => item.severity === 'error');
	}
	return items.filter((item) => item.category === filter);
}

function renderNotificationRows(items: NotificationItem[]): string[][] {
	return items.map((item, idx) => {
		const timestamp = item.timestamp.trim();
		return [
			String(idx + 1),
			truncate(item.category, 10),
			truncate(item.severity, 8),
			truncate(item.title, 32),
			timestamp === '' ? '-' : timestamp,
		];
	});
}

export async function executeInputCommand(ctx: CommandExecutorContext, apis: CommandAPIs = defaultAPIs): Promise<void> {
	const cmd = parseInputCommand(ctx.raw);

	switch (cmd.kind) {
	case 'noop':
		return;
	case 'chat':
		throw new Error('internal command router mismatch');
	case 'skill_invoke':
		throw new Error('internal command router mismatch');
	case 'invalid':
		ctx.pushErrorMessage(cmd.message);
		return;
	case 'quit':
		ctx.exit();
		return;
	case 'help':
		ctx.pushSystemMessage(commandHelpText());
		return;
	case 'sessions': {
		const sessions = await apis.listSessions(ctx.serverUrl);
		if (sessions.length === 0) {
			ctx.pushSystemMessage('(no sessions)');
			return;
		}
		ctx.pushSystemTable(['ID', 'TITLE'], renderSessionRows(sessions));
		return;
	}
	case 'new': {
		const created = await apis.createSession(ctx.serverUrl, cmd.title);
		ctx.setSessionID(created.id);
		clearResumeSelection(ctx);
		ctx.pushSystemMessage(`active session: ${created.id}`);
		return;
	}
	case 'resume': {
		await apis.getSession(ctx.serverUrl, cmd.sessionID);
		ctx.setSessionID(cmd.sessionID);
		ctx.pushSystemMessage(`resumed session: ${cmd.sessionID}`);
		clearResumeSelection(ctx);
		return;
	}
	case 'resume_select': {
		const sessions = await apis.listSessions(ctx.serverUrl);
		if (sessions.length === 0) {
			ctx.pushSystemMessage('(no sessions)');
			return;
		}
		ctx.setResumeCandidates(sessions);
		ctx.setResumeIndex(0);
		ctx.pushSystemMessage('Use ↑/↓ and Enter to select a session.');
		return;
	}
	case 'history': {
		const missing = missingSessionError(ctx.sessionID);
		if (missing !== null) {
			ctx.pushErrorMessage(missing);
			return;
		}
		const history = await apis.getHistory(ctx.serverUrl, ctx.sessionID);
		if (history.length === 0) {
			ctx.pushSystemMessage('(no history)');
			return;
		}
		ctx.pushSystemTable(
			['TIME', 'ROLE', 'CONTENT'],
			history.map((item) => [item.timestamp, item.role, truncate(item.content.replace(/\s+/g, ' '), 64)]),
		);
		return;
	}
	case 'export': {
		const missing = missingSessionError(ctx.sessionID);
		if (missing !== null) {
			ctx.pushErrorMessage(missing);
			return;
		}
		const markdown = await apis.exportSession(ctx.serverUrl, ctx.sessionID);
		ctx.pushSystemMessage(markdown);
		return;
	}
	case 'search': {
		const sessions = await apis.searchSessions(ctx.serverUrl, cmd.keyword);
		if (sessions.length === 0) {
			ctx.pushSystemMessage('(no sessions)');
			return;
		}
		ctx.pushSystemTable(['ID', 'TITLE'], renderSessionRows(sessions));
		return;
	}
	case 'status': {
		const status = await apis.getStatus(ctx.serverUrl);
		ctx.pushSystemMessage(`workspace=${status.workspace_dir} sessions=${status.session_count}`);
		return;
	}
	case 'compact': {
		const missing = missingSessionError(ctx.sessionID);
		if (missing !== null) {
			ctx.pushErrorMessage(missing);
			return;
		}
		const message = await apis.runCompact(ctx.serverUrl, ctx.sessionID);
		ctx.pushSystemMessage(message);
		return;
	}
	case 'heartbeat': {
		const response = await apis.runHeartbeatOnce(ctx.serverUrl);
		ctx.pushSystemMessage(response);
		return;
	}
	case 'skills': {
		const skills = await apis.listSkills(ctx.serverUrl);
		if (skills.length === 0) {
			ctx.pushSystemMessage('(no skills)');
			return;
		}
		ctx.pushSystemTable(['NAME', 'INVOKE', 'SOURCE', 'DESCRIPTION', 'RUNTIME_PATH'], renderSkillRows(skills));
		return;
	}
	case 'plugins': {
		const plugins = await apis.listPlugins(ctx.serverUrl);
		if (plugins.length === 0) {
			ctx.pushSystemMessage('(no plugins)');
			return;
		}
		ctx.pushSystemTable(['ID', 'NAME', 'SOURCE', 'VERSION', 'ROOT_DIR'], renderPluginRows(plugins));
		return;
	}
	case 'mcp': {
		const [servers, tools] = await Promise.all([
			apis.listMCPServers(ctx.serverUrl),
			apis.listMCPTools(ctx.serverUrl),
		]);
		if (servers.length === 0) {
			ctx.pushSystemMessage('(no mcp servers)');
		} else {
			ctx.pushSystemTable(['NAME', 'CONNECTED', 'TOOLS', 'COMMAND', 'ERROR'], renderMCPServerRows(servers));
		}
		if (tools.length === 0) {
			ctx.pushSystemMessage('(no mcp tools)');
		} else {
			ctx.pushSystemTable(['SERVER', 'NAME', 'DESCRIPTION'], renderMCPToolRows(tools));
		}
		return;
	}
	case 'reload': {
		const result = await apis.reloadExtensions(ctx.serverUrl);
		if (!result.reloaded) {
			ctx.pushErrorMessage('extensions reload failed');
			return;
		}
		ctx.pushSystemMessage(
			`extensions reloaded: version=${result.version ?? '-'} skills=${result.skills ?? 0} plugins=${result.plugins ?? 0} mcp=${result.mcp_count ?? 0} gateway_refresh=${result.gateway_refreshed ? 'yes' : 'no'} gateway_agents=${result.gateway_agents ?? 0}`,
		);
		return;
	}
	case 'agents': {
		const agents = await apis.listAgents(ctx.serverUrl);
		if (agents.length === 0) {
			ctx.pushSystemMessage('(no agents)');
			return;
		}
		const detail = cmd.detail === true;
		if (detail) {
			ctx.pushSystemTable(['NAME', 'DEFAULT', 'ENABLED', 'KIND', 'POLICY', 'ALLOW', 'SOURCE', 'ENTRY', 'DESCRIPTION'], renderAgentRows(agents, true));
			return;
		}
		ctx.pushSystemTable(['NAME', 'DEFAULT', 'ENABLED', 'KIND', 'DESCRIPTION'], renderAgentRows(agents, false));
		return;
	}
	case 'runs': {
		const runs = await apis.listAgentRuns(ctx.serverUrl, 30);
		if (runs.length === 0) {
			ctx.pushSystemMessage('(no runs)');
			return;
		}
		ctx.pushSystemTable(['RUN_ID', 'SESSION', 'STATUS', 'AGENT', 'DETAIL'], renderRunRows(runs));
		return;
	}
	case 'spawn': {
		const payload: {session_id?: string; title?: string; message: string; agent?: string} = {
			message: cmd.message,
		};
		if ((cmd.sessionID ?? '').trim() !== '') {
			payload.session_id = (cmd.sessionID ?? '').trim();
		} else if (ctx.sessionID.trim() !== '') {
			payload.session_id = ctx.sessionID.trim();
		}
		if ((cmd.title ?? '').trim() !== '') {
			payload.title = (cmd.title ?? '').trim();
		}
		if ((cmd.agent ?? '').trim() !== '') {
			payload.agent = (cmd.agent ?? '').trim();
		}
		const run = await apis.spawnAgentRun(ctx.serverUrl, payload);
		if (cmd.wait) {
			const deadline = Date.now() + 30_000;
			let current = run;
			while (isRunPending(current.status)) {
				if (Date.now() > deadline) {
					ctx.pushErrorMessage(`run wait timeout: ${current.run_id} status=${current.status}`);
					return;
				}
				current = await apis.getAgentRun(ctx.serverUrl, current.run_id);
				if (isRunPending(current.status)) {
					await new Promise<void>((resolve) => {
						setTimeout(resolve, 250);
					});
				}
			}
			if (current.status.trim().toLowerCase() === 'completed') {
				ctx.pushSystemMessage(`run completed: ${current.run_id} status=${current.status}`);
			} else {
				const detail = (current.error ?? '').trim();
				if (detail === '') {
					ctx.pushErrorMessage(`run finished: ${current.run_id} status=${current.status}`);
				} else {
					ctx.pushErrorMessage(`run finished: ${current.run_id} status=${current.status} error=${truncate(detail, 80)}`);
				}
			}
			return;
		}
		ctx.pushSystemMessage(`run accepted: ${run.run_id} status=${run.status}`);
		return;
	}
	case 'run': {
		const run = await apis.getAgentRun(ctx.serverUrl, cmd.runID);
		ctx.pushSystemTable(['FIELD', 'VALUE'], [
			['run_id', run.run_id],
			['session_id', run.session_id ?? '-'],
			['status', run.status],
			['agent', run.agent ?? '-'],
			['created_at', run.created_at ?? '-'],
			['started_at', run.started_at ?? '-'],
			['completed_at', run.completed_at ?? '-'],
			['response', truncate(run.response ?? '', 80)],
			['error', truncate(run.error ?? '', 80)],
		]);
		return;
	}
	case 'cancel_run': {
		const run = await apis.cancelAgentRun(ctx.serverUrl, cmd.runID);
		ctx.pushSystemMessage(`run canceled: ${run.run_id} status=${run.status}`);
		return;
	}
	case 'gateway': {
		let status: GatewayStatus;
		if (cmd.action === 'reload') {
			status = await apis.reloadGateway(ctx.serverUrl);
		} else if (cmd.action === 'restart') {
			status = await apis.restartGateway(ctx.serverUrl);
		} else {
			status = await apis.getGatewayStatus(ctx.serverUrl);
		}
		ctx.pushSystemTable(['FIELD', 'VALUE'], renderGatewayRows(status));
		return;
	}
	case 'sentinel': {
		let status: SentinelStatus;
		if (cmd.action === 'restart') {
			status = await apis.restartSentinel(ctx.casedServerUrl);
		} else if (cmd.action === 'pause') {
			status = await apis.pauseSentinel(ctx.casedServerUrl);
		} else if (cmd.action === 'resume') {
			status = await apis.resumeSentinel(ctx.casedServerUrl);
		} else if (cmd.action === 'events') {
			const events = await apis.listSentinelEvents(ctx.casedServerUrl, cmd.limit ?? 20);
			if (events.length === 0) {
				ctx.pushSystemMessage('(no sentinel events)');
				return;
			}
			ctx.pushSystemTable(['ID', 'TIME', 'LEVEL', 'TYPE', 'MESSAGE'], renderSentinelEventRows(events));
			return;
		} else {
			status = await apis.getSentinelStatus(ctx.casedServerUrl);
		}
		ctx.pushSystemTable(['FIELD', 'VALUE'], renderSentinelRows(status));
		return;
	}
	case 'channels': {
		const status = await apis.getGatewayStatus(ctx.serverUrl);
		ctx.pushSystemTable(['FIELD', 'VALUE'], [
			['channels_local', status.channels_local_enabled ? 'yes' : 'no'],
			['channels_webhook', status.channels_webhook_enabled ? 'yes' : 'no'],
			['channels_telegram', status.channels_telegram_enabled ? 'yes' : 'no'],
			['gateway_enabled', status.enabled ? 'yes' : 'no'],
		]);
		return;
	}
	case 'cron_list': {
		const jobs = await apis.listCronJobs(ctx.serverUrl);
		if (jobs.length === 0) {
			ctx.pushSystemMessage('(no cron jobs)');
			return;
		}
		ctx.pushSystemTable(['ID', 'NAME', 'SCHEDULE', 'ENABLED'], renderCronRows(jobs));
		return;
	}
	case 'cron_add': {
		const created = await apis.createCronJob(ctx.serverUrl, {
			schedule: cmd.schedule,
			prompt: cmd.prompt,
		});
		ctx.pushSystemMessage(`cron job created: ${created.id}`);
		return;
	}
	case 'cron_run': {
		const response = await apis.runCronJob(ctx.serverUrl, cmd.jobID);
		ctx.pushSystemMessage(response);
		return;
	}
	case 'cron_get': {
		const job = await apis.getCronJob(ctx.serverUrl, cmd.jobID);
		ctx.pushSystemTable(['FIELD', 'VALUE'], renderCronDetailRows(job));
		return;
	}
	case 'cron_runs': {
		const runs = await apis.listCronRuns(ctx.serverUrl, cmd.jobID, cmd.limit);
		if (runs.length === 0) {
			ctx.setCronRunsPreview([`job=${cmd.jobID}`, '(no runs)']);
			ctx.pushSystemMessage(`(no runs for cron job: ${cmd.jobID})`);
			return;
		}
		ctx.setCronRunsPreview([`job=${cmd.jobID}`, ...renderCronRunPreviewLines(runs)]);
		ctx.pushSystemTable(['TIME', 'STATUS', 'DETAIL'], renderCronRunRows(runs));
		return;
	}
	case 'cron_delete': {
		await apis.deleteCronJob(ctx.serverUrl, cmd.jobID);
		ctx.pushSystemMessage(`cron job deleted: ${cmd.jobID}`);
		return;
	}
	case 'cron_enable': {
		await apis.updateCronJob(ctx.serverUrl, cmd.jobID, {enabled: true});
		ctx.pushSystemMessage(`cron job enabled: ${cmd.jobID}`);
		return;
	}
	case 'cron_disable': {
		await apis.updateCronJob(ctx.serverUrl, cmd.jobID, {enabled: false});
		ctx.pushSystemMessage(`cron job disabled: ${cmd.jobID}`);
		return;
	}
	case 'notify_list': {
		const filter = ctx.getNotificationFilter();
		const filtered = filterNotifications(ctx.getNotifications(), filter);
		if (filtered.length === 0) {
			ctx.pushSystemMessage('(no notifications)');
			return;
		}
		ctx.pushSystemTable(['#', 'CATEGORY', 'SEVERITY', 'TITLE', 'TIME'], renderNotificationRows(filtered));
		ctx.markNotificationsSeen();
		return;
	}
	case 'notify_filter': {
		ctx.setNotificationFilter(cmd.filter);
		ctx.pushSystemMessage(`notification filter: ${cmd.filter}`);
		return;
	}
	case 'notify_open': {
		const filter = ctx.getNotificationFilter();
		const filtered = filterNotifications(ctx.getNotifications(), filter);
		const item = filtered[cmd.index - 1];
		if (item === undefined) {
			ctx.pushErrorMessage(`notification not found: ${cmd.index}`);
			return;
		}
		const lines = [
			`[${item.category}/${item.severity}] ${item.title}`,
			item.message,
			item.timestamp !== '' ? `time: ${item.timestamp}` : '',
		].filter((line) => line.trim() !== '');
		ctx.pushSystemMessage(lines.join(' | '));
		ctx.markNotificationsSeen();
		return;
	}
	case 'notify_clear': {
		ctx.clearNotifications();
		ctx.markNotificationsSeen();
		ctx.pushSystemMessage('notifications cleared');
		return;
	}
	}
}
