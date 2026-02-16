type PushOptions = {
	prepend: boolean;
	accumulate?: boolean;
};

export class KillRing {
	private readonly maxItems: number;
	private ring: string[] = [];

	constructor(maxItems = 30) {
		this.maxItems = Math.max(1, maxItems);
	}

	push(text: string, options: PushOptions): void {
		if (text === '') {
			return;
		}
		if (options.accumulate && this.ring.length > 0) {
			const last = this.ring.pop() ?? '';
			this.ring.push(options.prepend ? text + last : last + text);
			return;
		}
		this.ring.push(text);
		if (this.ring.length > this.maxItems) {
			this.ring.shift();
		}
	}

	peek(): string | undefined {
		if (this.ring.length === 0) {
			return undefined;
		}
		return this.ring[this.ring.length - 1];
	}

	rotate(): void {
		if (this.ring.length <= 1) {
			return;
		}
		const last = this.ring.pop();
		if (last === undefined) {
			return;
		}
		this.ring.unshift(last);
	}

	clear(): void {
		this.ring = [];
	}

	get length(): number {
		return this.ring.length;
	}
}
