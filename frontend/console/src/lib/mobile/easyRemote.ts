// The vibe-mode calls aimed at one desktop (docs/plans/vibe-mode-redesign
// unit 8): the same EasyAPI the console uses, each request sent to the
// desktop with the phone's token. The desk holds the phone to the easy
// routes in its focus table (unit 6); preview and delete answer 403.
import { requestJSONAt, type APITarget } from '../api/client.ts'
import { easyApiOf, type EasyAPI } from '../api/easy.ts'

// timeoutMs bounds every request: the project list reads several desktops
// at once, and one asleep should show as not answering instead of holding
// the list.
export function remoteEasyApi(target: APITarget, options: { timeoutMs?: number } = {}): EasyAPI {
  return easyApiOf(<T>(path: string, init?: RequestInit) => {
    const bounded = options.timeoutMs && !init?.signal ? { ...init, signal: AbortSignal.timeout(options.timeoutMs) } : init
    return requestJSONAt<T>(target, path, bounded)
  })
}
