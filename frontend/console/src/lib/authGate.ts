import type { AuthWhoamiResponse } from './types'

// Whether the console has to show the login form for this whoami answer.
// Only a server in `required` mode asks a browser to sign in; with auth off
// every caller is already the admin and no password account need exist.
export function loginRequiredFor(auth: AuthWhoamiResponse | null | undefined): boolean {
  if (!auth) return false
  return auth.auth_mode === 'required' && !auth.authenticated
}

// Whether signing out can do anything. With auth off there is no browser
// session to end, so the header offers no sign-out.
export function canSignOut(auth: AuthWhoamiResponse | null | undefined): boolean {
  if (!auth) return false
  return auth.auth_mode !== 'off'
}
