import test from 'node:test';
import assert from 'node:assert/strict';
import {computeChatWindow, nextChatScrollOffset, nextResumeIndex, resolveKeyAction, tailLines, toolLinesFromStatusEvent} from './view.js';

test('toolLinesFromStatusEvent returns lifecycle header and previews', () => {
	assert.deepEqual(
		toolLinesFromStatusEvent({phase: 'before_tool_call', tool_name: 'session_status', tool_call_id: 'call_1', tool_args_preview: '{"query":"coffee"}'}),
		['start session_status #call_1', 'args {"query":"coffee"}'],
	);
	assert.deepEqual(
		toolLinesFromStatusEvent({phase: 'after_tool_call', tool_name: 'session_status', tool_call_id: 'call_1', tool_result_preview: '{"ok":true}'}),
		['done session_status #call_1', 'result {"ok":true}'],
	);
	assert.deepEqual(toolLinesFromStatusEvent({phase: 'error', message: 'failed'}), ['error failed']);
	assert.deepEqual(toolLinesFromStatusEvent({phase: 'before_llm'}), []);
});

test('resolveKeyAction prioritizes exit and resume navigation', () => {
	assert.equal(resolveKeyAction('c', {ctrl: true, upArrow: false, downArrow: false, pageUp: false, pageDown: false}, false), 'exit');
	assert.equal(resolveKeyAction('', {ctrl: false, upArrow: true, downArrow: false, pageUp: false, pageDown: false}, true), 'resume_up');
	assert.equal(resolveKeyAction('', {ctrl: false, upArrow: false, downArrow: true, pageUp: false, pageDown: false}, true), 'resume_down');
	assert.equal(resolveKeyAction('u', {ctrl: true, upArrow: false, downArrow: false, pageUp: false, pageDown: false}, false), 'chat_page_up');
	assert.equal(resolveKeyAction('u', {ctrl: true, upArrow: false, downArrow: false, pageUp: false, pageDown: false}, false, 'hello'), 'none');
	assert.equal(resolveKeyAction('d', {ctrl: true, upArrow: false, downArrow: false, pageUp: false, pageDown: false}, false), 'chat_page_down');
});

test('nextResumeIndex wraps in both directions', () => {
	assert.equal(nextResumeIndex(0, 3, 'up'), 2);
	assert.equal(nextResumeIndex(2, 3, 'down'), 0);
	assert.equal(nextResumeIndex(0, 0, 'up'), 0);
});

test('nextChatScrollOffset applies page and lower bound', () => {
	assert.equal(nextChatScrollOffset(0, 20, 'up'), 20);
	assert.equal(nextChatScrollOffset(10, 20, 'down'), 0);
	assert.equal(nextChatScrollOffset(30, 20, 'down'), 10);
});

test('computeChatWindow clamps effective offset', () => {
	const window = computeChatWindow(30, 20, 50);
	assert.equal(window.maxOffset, 10);
	assert.equal(window.effectiveOffset, 10);
	assert.equal(window.chatStart, 0);
	assert.equal(window.chatEnd, 20);
});

test('tailLines returns trailing lines', () => {
	assert.deepEqual(tailLines(['a', 'b', 'c'], 2), ['b', 'c']);
	assert.deepEqual(tailLines(['a', 'b'], 5), ['a', 'b']);
});
