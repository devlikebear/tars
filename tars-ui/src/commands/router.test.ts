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
	assert.deepEqual(parseInputCommand('/cron delete job_123'), {kind: 'cron_delete', jobID: 'job_123'});
	assert.deepEqual(parseInputCommand('/cron enable job_123'), {kind: 'cron_enable', jobID: 'job_123'});
	assert.deepEqual(parseInputCommand('/cron disable job_123'), {kind: 'cron_disable', jobID: 'job_123'});
	assert.deepEqual(parseInputCommand('/notify'), {kind: 'notify_list'});
	assert.deepEqual(parseInputCommand('/notify list'), {kind: 'notify_list'});
	assert.deepEqual(parseInputCommand('/notify filter cron'), {kind: 'notify_filter', filter: 'cron'});
	assert.deepEqual(parseInputCommand('/notify open 2'), {kind: 'notify_open', index: 2});
	assert.deepEqual(parseInputCommand('/notify clear'), {kind: 'notify_clear'});
	assert.deepEqual(parseInputCommand('／help'), {kind: 'help'});
	assert.deepEqual(parseInputCommand('\\sessions'), {kind: 'sessions'});
});

test('router returns invalid for malformed or unknown command', () => {
	assert.deepEqual(parseInputCommand('/search'), {kind: 'invalid', message: 'usage: /search {keyword}'});
	assert.deepEqual(parseInputCommand('/cron add'), {kind: 'invalid', message: 'usage: /cron add {schedule} {prompt}'});
	assert.deepEqual(parseInputCommand('/cron run'), {kind: 'invalid', message: 'usage: /cron run {job_id}'});
	assert.deepEqual(parseInputCommand('/cron delete'), {kind: 'invalid', message: 'usage: /cron delete {job_id}'});
	assert.deepEqual(parseInputCommand('/cron enable'), {kind: 'invalid', message: 'usage: /cron enable {job_id}'});
	assert.deepEqual(parseInputCommand('/cron disable'), {kind: 'invalid', message: 'usage: /cron disable {job_id}'});
	assert.deepEqual(parseInputCommand('/notify filter'), {kind: 'invalid', message: 'usage: /notify filter {all|cron|heartbeat|error}'});
	assert.deepEqual(parseInputCommand('/notify filter foo'), {kind: 'invalid', message: 'usage: /notify filter {all|cron|heartbeat|error}'});
	assert.deepEqual(parseInputCommand('/notify open'), {kind: 'invalid', message: 'usage: /notify open {index}'});
	assert.deepEqual(parseInputCommand('/notify open xx'), {kind: 'invalid', message: 'usage: /notify open {index}'});
	assert.deepEqual(parseInputCommand('/what'), {kind: 'invalid', message: 'unknown command: /what'});
	assert.deepEqual(parseInputCommand('   '), {kind: 'noop'});
});
