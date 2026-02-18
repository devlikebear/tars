type ClientContext = {
	apiToken: string;
	adminApiToken: string;
	casedApiToken: string;
	casedAdminApiToken: string;
	workspaceId: string;
};

const context: ClientContext = {
	apiToken: '',
	adminApiToken: '',
	casedApiToken: '',
	casedAdminApiToken: '',
	workspaceId: '',
};

export function configureAPIClientContext(next: Partial<ClientContext>): void {
	context.apiToken = (next.apiToken ?? '').trim();
	context.adminApiToken = (next.adminApiToken ?? '').trim();
	context.casedApiToken = (next.casedApiToken ?? '').trim();
	context.casedAdminApiToken = (next.casedAdminApiToken ?? '').trim();
	context.workspaceId = (next.workspaceId ?? '').trim();
}

function mergeHeaders(
	baseToken: string,
	extra?: Record<string, string>,
): Record<string, string> | undefined {
	const out: Record<string, string> = {};
	const token = baseToken.trim();
	if (token !== '') {
		out.Authorization = `Bearer ${token}`;
	}
	const workspaceID = context.workspaceId.trim();
	if (workspaceID !== '') {
		out['Tars-Workspace-Id'] = workspaceID;
	}
	if (extra !== undefined) {
		for (const [key, value] of Object.entries(extra)) {
			const trimmedKey = key.trim();
			if (trimmedKey === '') {
				continue;
			}
			out[trimmedKey] = value;
		}
	}
	if (Object.keys(out).length === 0) {
		return undefined;
	}
	return out;
}

export function tarsHeaders(extra?: Record<string, string>): Record<string, string> | undefined {
	return mergeHeaders(context.apiToken, extra);
}

export function casedHeaders(extra?: Record<string, string>): Record<string, string> | undefined {
	return mergeHeaders(context.casedApiToken, extra);
}

export function tarsAdminHeaders(extra?: Record<string, string>): Record<string, string> | undefined {
	const token = context.adminApiToken !== '' ? context.adminApiToken : context.apiToken;
	return mergeHeaders(token, extra);
}

export function casedAdminHeaders(extra?: Record<string, string>): Record<string, string> | undefined {
	const token = context.casedAdminApiToken !== '' ? context.casedAdminApiToken : context.casedApiToken;
	return mergeHeaders(token, extra);
}
