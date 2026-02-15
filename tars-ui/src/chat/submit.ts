import {SessionSummary} from '../types.js';

export type SubmitInputContext = {
	input: string;
	busy: boolean;
	resumeCandidates: SessionSummary[] | null;
	resumeIndex: number;
	setInput: (value: string) => void;
	setSessionID: (sessionID: string) => void;
	setResumeCandidates: (sessions: SessionSummary[] | null) => void;
	setResumeIndex: (index: number) => void;
	pushSystemMessage: (text: string) => void;
	pushErrorMessage: (text: string) => void;
	pushDebug: (text: string) => void;
	executeCommand: (line: string) => Promise<void>;
	sendChat: (line: string) => Promise<void>;
};

function clearResumeSelection(ctx: SubmitInputContext): void {
	ctx.setResumeCandidates(null);
	ctx.setResumeIndex(0);
}

function highlightedCandidate(candidates: SessionSummary[], resumeIndex: number): SessionSummary | null {
	return candidates[resumeIndex] ?? candidates[0] ?? null;
}

export async function submitInput(ctx: SubmitInputContext): Promise<void> {
	const line = ctx.input.trim();
	if (line === '' && ctx.resumeCandidates === null) {
		return;
	}
	if (ctx.busy) {
		return;
	}
	ctx.setInput('');

	if (ctx.resumeCandidates !== null) {
		if (line === '') {
			const selected = highlightedCandidate(ctx.resumeCandidates, ctx.resumeIndex);
			if (selected === null) {
				return;
			}
			ctx.setSessionID(selected.id);
			ctx.pushSystemMessage(`resumed session: ${selected.id}`);
			clearResumeSelection(ctx);
			return;
		}

		const choice = Number.parseInt(line, 10);
		if (Number.isNaN(choice) || choice < 1 || choice > ctx.resumeCandidates.length) {
			ctx.pushErrorMessage('invalid selection');
			return;
		}

		const selected = ctx.resumeCandidates[choice - 1];
		if (!selected) {
			ctx.pushErrorMessage('invalid selection');
			return;
		}
		ctx.setSessionID(selected.id);
		clearResumeSelection(ctx);
		ctx.pushSystemMessage(`resumed session: ${selected.id}`);
		return;
	}

	if (line.startsWith('/')) {
		try {
			await ctx.executeCommand(line);
		} catch (err) {
			const message = err instanceof Error ? err.message : String(err);
			ctx.pushErrorMessage(message);
			ctx.pushDebug(`command error: ${message}`);
		}
		return;
	}

	await ctx.sendChat(line);
}
