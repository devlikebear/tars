import test from 'node:test';
import assert from 'node:assert/strict';
import {completeCommandInput} from './complete.js';

test('completeCommandInput completes root slash commands with tab', () => {
	assert.equal(completeCommandInput('/he'), '/he');
	assert.equal(completeCommandInput('/sess'), '/sessions ');
});

test('completeCommandInput leaves unknown command untouched', () => {
	assert.equal(completeCommandInput('/zzz'), '/zzz');
	assert.equal(completeCommandInput('hello'), 'hello');
});

test('completeCommandInput completes cron subcommands', () => {
	assert.equal(completeCommandInput('/cron r'), '/cron run');
	assert.equal(completeCommandInput('/cron ru'), '/cron run');
	assert.equal(completeCommandInput('/cron run'), '/cron run ');
	assert.equal(completeCommandInput('/cron de'), '/cron delete ');
});

test('completeCommandInput completes notify subcommands', () => {
	assert.equal(completeCommandInput('/notify cl'), '/notify clear ');
	assert.equal(completeCommandInput('/notify fi'), '/notify filter ');
});
