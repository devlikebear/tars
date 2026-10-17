import { derived, get, writable } from 'svelte/store'
import { en } from './en'
import { ko } from './ko'
import { resolveInitialLocale, STORAGE_KEY } from './locale'
import type { Locale, Translations } from './types'

const translations: Record<Locale, Translations> = { en, ko }

function detectInitialLocale(): Locale {
  if (typeof window === 'undefined') return 'en'
  try {
    return resolveInitialLocale(window.localStorage.getItem(STORAGE_KEY), window.navigator.language)
  } catch {
    // Ignore storage failures and fall back to browser language.
  }
  return resolveInitialLocale(undefined, window.navigator.language)
}

export const locale = writable<Locale>(detectInitialLocale())
export const t = derived(locale, ($locale) => translations[$locale] ?? en)

function syncDocumentLang(next: Locale) {
  if (typeof document === 'undefined') return
  document.documentElement.lang = next
}

// Keeps <html lang> in step with every locale change, including the
// detected initial one — not just explicit setLocale() calls — so a
// screen reader or browser spellchecker sees the console's real language
// from first paint, not a stale "en" left over from #701 (tars#1241).
locale.subscribe(syncDocumentLang)

export function setLocale(next: Locale) {
  locale.set(next)
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(STORAGE_KEY, next)
  } catch {
    // Ignore storage failures; the current session still updates.
  }
}

export function getLocale(): Locale {
  return get(locale)
}

export const locales: Locale[] = ['en', 'ko']
