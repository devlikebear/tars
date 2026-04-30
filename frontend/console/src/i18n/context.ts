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
