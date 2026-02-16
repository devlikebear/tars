export class UndoStack<S> {
	private readonly maxItems: number;
	private readonly stack: S[] = [];

	constructor(maxItems = 120) {
		this.maxItems = Math.max(1, maxItems);
	}

	push(state: S): void {
		const clone = structuredClone(state);
		this.stack.push(clone);
		if (this.stack.length > this.maxItems) {
			this.stack.shift();
		}
	}

	pop(): S | undefined {
		return this.stack.pop();
	}

	clear(): void {
		this.stack.length = 0;
	}

	get length(): number {
		return this.stack.length;
	}
}
