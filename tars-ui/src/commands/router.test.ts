import test from 'node:test';
import assert from 'node:assert/strict';
import {parseInputCommand} from './router.js';

test('router parses chat and quit commands', () => {
	assert.deepEqual(parseInputCommand('hello'), {kind: 'chat', message: 'hello'});
	assert.deepEqual(parseInputCommand('/quit'), {kind: 'quit'});
	assert.deepEqual(parseInputCommand('/exit'), {kind: 'quit'});
	assert.deepEqual(parseInputCommand('₩resume sess-kor'), {kind: 'resume', sessionID: 'sess-kor'});
});

test('router parses slash command options', () => {
	assert.deepEqual(parseInputCommand('/new'), {kind: 'new', title: 'chat'});
	assert.deepEqual(parseInputCommand('/new my title'), {kind: 'new', title: 'my title'});
	assert.deepEqual(parseInputCommand('/resume abc'), {kind: 'resume', sessionID: 'abc'});
	assert.deepEqual(parseInputCommand('/resume'), {kind: 'resume_select'});
	assert.deepEqual(parseInputCommand('/search keyword'), {kind: 'search', keyword: 'keyword'});
	assert.deepEqual(parseInputCommand('/cron'), {kind: 'cron_list'});
	assert.deepEqual(parseInputCommand('/cron list'), {kind: 'cron_list'});
	assert.deepEqual(parseInputCommand('/cron add every:1h check inbox'), {kind: 'cron_add', schedule: 'every:1h', prompt: 'check inbox'});
	assert.deepEqual(parseInputCommand('/cron run job_123'), {kind: 'cron_run', jobID: 'job_123'});
	assert.deepEqual(parseInputCommand('/cron get job_123'), {kind: 'cron_get', jobID: 'job_123'});
	assert.deepEqual(parseInputCommand('/cron runs job_123'), {kind: 'cron_runs', jobID: 'job_123', limit: 20});
	assert.deepEqual(parseInputCommand('/cron runs job_123 5'), {kind: 'cron_runs', jobID: 'job_123', limit: 5});
	assert.deepEqual(parseInputCommand('/cron delete job_123'), {kind: 'cron_delete', jobID: 'job_123'});
	assert.deepEqual(parseInputCommand('/cron enable job_123'), {kind: 'cron_enable', jobID: 'job_123'});
	assert.deepEqual(parseInputCommand('/cron disable job_123'), {kind: 'cron_disable', jobID: 'job_123'});
	assert.deepEqual(parseInputCommand('/notify'), {kind: 'notify_list'});
	assert.deepEqual(parseInputCommand('/notify list'), {kind: 'notify_list'});
	assert.deepEqual(parseInputCommand('/notify filter cron'), {kind: 'notify_filter', filter: 'cron'});
	assert.deepEqual(parseInputCommand('/notify open 2'), {kind: 'notify_open', index: 2});
	assert.deepEqual(parseInputCommand('/notify clear'), {kind: 'notify_clear'});
	assert.deepEqual(parseInputCommand('/skills'), {kind: 'skills'});
	assert.deepEqual(parseInputCommand('/plugins'), {kind: 'plugins'});
	assert.deepEqual(parseInputCommand('/mcp'), {kind: 'mcp'});
	assert.deepEqual(parseInputCommand('/reload'), {kind: 'reload'});
	assert.deepEqual(parseInputCommand('/agents'), {kind: 'agents'});
	assert.deepEqual(parseInputCommand('/agents --detail'), {kind: 'agents', detail: true});
	assert.deepEqual(parseInputCommand('/runs'), {kind: 'runs'});
	assert.deepEqual(parseInputCommand('/spawn hello worker'), {kind: 'spawn', message: 'hello worker'});
	assert.deepEqual(parseInputCommand('/spawn --agent worker hello worker'), {kind: 'spawn', message: 'hello worker', agent: 'worker'});
	assert.deepEqual(parseInputCommand('/spawn --title nightly hello worker'), {kind: 'spawn', message: 'hello worker', title: 'nightly'});
	assert.deepEqual(parseInputCommand('/spawn --session sess_123 hello worker'), {kind: 'spawn', message: 'hello worker', sessionID: 'sess_123'});
	assert.deepEqual(parseInputCommand('/spawn --wait hello worker'), {kind: 'spawn', message: 'hello worker', wait: true});
	assert.deepEqual(parseInputCommand('/spawn --agent worker --title nightly --session sess_123 hello worker'), {
		kind: 'spawn',
		message: 'hello worker',
		agent: 'worker',
		title: 'nightly',
		sessionID: 'sess_123',
	});
	assert.deepEqual(parseInputCommand('/spawn --wait --agent worker hello worker'), {
		kind: 'spawn',
		message: 'hello worker',
		agent: 'worker',
		wait: true,
	});
	assert.deepEqual(parseInputCommand('/run run_1'), {kind: 'run', runID: 'run_1'});
	assert.deepEqual(parseInputCommand('/cancel-run run_1'), {kind: 'cancel_run', runID: 'run_1'});
	assert.deepEqual(parseInputCommand('/gateway'), {kind: 'gateway', action: 'status'});
	assert.deepEqual(parseInputCommand('/gateway reload'), {kind: 'gateway', action: 'reload'});
	assert.deepEqual(parseInputCommand('/gateway restart'), {kind: 'gateway', action: 'restart'});
	assert.deepEqual(parseInputCommand('/sentinel'), {kind: 'sentinel', action: 'status'});
	assert.deepEqual(parseInputCommand('/sentinel status'), {kind: 'sentinel', action: 'status'});
	assert.deepEqual(parseInputCommand('/sentinel restart'), {kind: 'sentinel', action: 'restart'});
	assert.deepEqual(parseInputCommand('/sentinel pause'), {kind: 'sentinel', action: 'pause'});
	assert.deepEqual(parseInputCommand('/sentinel resume'), {kind: 'sentinel', action: 'resume'});
	assert.deepEqual(parseInputCommand('/sentinel events'), {kind: 'sentinel', action: 'events', limit: 20});
	assert.deepEqual(parseInputCommand('/sentinel events 10'), {kind: 'sentinel', action: 'events', limit: 10});
	assert.deepEqual(parseInputCommand('/channels'), {kind: 'channels'});
	assert.deepEqual(parseInputCommand('/deploy now'), {kind: 'skill_invoke', skillName: 'deploy', message: '/deploy now'});
	assert.deepEqual(parseInputCommand('／help'), {kind: 'help'});
	assert.deepEqual(parseInputCommand('\\sessions'), {kind: 'sessions'});
});

test('router returns invalid for malformed or unknown command', () => {
	assert.deepEqual(parseInputCommand('/search'), {kind: 'invalid', message: 'usage: /search {keyword}'});
	assert.deepEqual(parseInputCommand('/cron add'), {kind: 'invalid', message: 'usage: /cron add {schedule} {prompt}'});
	assert.deepEqual(parseInputCommand('/cron run'), {kind: 'invalid', message: 'usage: /cron run {job_id}'});
	assert.deepEqual(parseInputCommand('/cron get'), {kind: 'invalid', message: 'usage: /cron get {job_id}'});
	assert.deepEqual(parseInputCommand('/cron runs'), {kind: 'invalid', message: 'usage: /cron runs {job_id} [limit]'});
	assert.deepEqual(parseInputCommand('/cron runs job_123 0'), {kind: 'invalid', message: 'usage: /cron runs {job_id} [limit]'});
	assert.deepEqual(parseInputCommand('/cron runs job_123 xx'), {kind: 'invalid', message: 'usage: /cron runs {job_id} [limit]'});
	assert.deepEqual(parseInputCommand('/cron delete'), {kind: 'invalid', message: 'usage: /cron delete {job_id}'});
	assert.deepEqual(parseInputCommand('/cron enable'), {kind: 'invalid', message: 'usage: /cron enable {job_id}'});
	assert.deepEqual(parseInputCommand('/cron disable'), {kind: 'invalid', message: 'usage: /cron disable {job_id}'});
	assert.deepEqual(parseInputCommand('/notify filter'), {kind: 'invalid', message: 'usage: /notify filter {all|cron|heartbeat|error}'});
	assert.deepEqual(parseInputCommand('/notify filter foo'), {kind: 'invalid', message: 'usage: /notify filter {all|cron|heartbeat|error}'});
	assert.deepEqual(parseInputCommand('/notify open'), {kind: 'invalid', message: 'usage: /notify open {index}'});
	assert.deepEqual(parseInputCommand('/notify open xx'), {kind: 'invalid', message: 'usage: /notify open {index}'});
	assert.deepEqual(parseInputCommand('/run'), {kind: 'invalid', message: 'usage: /run {run_id}'});
	assert.deepEqual(parseInputCommand('/spawn'), {kind: 'invalid', message: 'usage: /spawn [--agent {name}] [--title {title}] [--session {id}] [--wait] {message}'});
	assert.deepEqual(parseInputCommand('/spawn --agent'), {kind: 'invalid', message: 'usage: /spawn [--agent {name}] [--title {title}] [--session {id}] [--wait] {message}'});
	assert.deepEqual(parseInputCommand('/spawn --unknown hello'), {kind: 'invalid', message: 'usage: /spawn [--agent {name}] [--title {title}] [--session {id}] [--wait] {message}'});
	assert.deepEqual(parseInputCommand('/cancel-run'), {kind: 'invalid', message: 'usage: /cancel-run {run_id}'});
	assert.deepEqual(parseInputCommand('/gateway xx'), {kind: 'invalid', message: 'usage: /gateway {status|reload|restart}'});
	assert.deepEqual(parseInputCommand('/sentinel xx'), {kind: 'invalid', message: 'usage: /sentinel {status|restart|pause|resume|events [limit]}'});
	assert.deepEqual(parseInputCommand('/sentinel events xx'), {kind: 'invalid', message: 'usage: /sentinel {status|restart|pause|resume|events [limit]}'});
	assert.deepEqual(parseInputCommand('/agents foo'), {kind: 'invalid', message: 'usage: /agents [--detail]'});
	assert.deepEqual(parseInputCommand('/what'), {kind: 'skill_invoke', skillName: 'what', message: '/what'});
	assert.deepEqual(parseInputCommand('   '), {kind: 'noop'});
});
