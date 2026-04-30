import type { Locale } from './types'

export const STORAGE_KEY = 'tars_console_locale'

export function normalizeLocale(raw?: string | null): Locale | '' {
  const value = raw?.trim().toLowerCase()
  if (!value) return ''
  if (value.startsWith('ko')) return 'ko'
  if (value.startsWith('en')) return 'en'
  return ''
}

export function resolveInitialLocale(stored?: string | null, browserLanguage?: string | null): Locale {
  return normalizeLocale(stored) || normalizeLocale(browserLanguage) || 'en'
}
