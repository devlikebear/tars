import {createSession, exportSession, getHistory, getSession, listSessions, searchSessions} from '../api/session.js';
import {createCronJob, deleteCronJob, getCronJob, listCronJobs, listCronRuns, runCronJob, updateCronJob} from '../api/cron.js';
import {listMCPServers, listMCPTools, listPlugins, listSkills, reloadExtensions} from '../api/extensions.js';
import {getStatus, runCompact, runHeartbeatOnce} from '../api/system.js';
import {parseInputCommand} from '../commands/router.js';
import {CronJob, CronRunRecord, MCPServerStatus, MCPToolInfo, NotificationItem, NotificationFilter, PluginDefinition, SessionHistoryItem, SessionSummary, SkillDefinition} from '../types.js';
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
	reloadExtensions: (serverUrl: string) => Promise<{reloaded: boolean; version?: number; skills?: number; plugins?: number; mcp_count?: number}>;
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
};

export type CommandExecutorContext = {
	raw: string;
	serverUrl: string;
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
			`extensions reloaded: version=${result.version ?? '-'} skills=${result.skills ?? 0} plugins=${result.plugins ?? 0} mcp=${result.mcp_count ?? 0}`,
		);
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
