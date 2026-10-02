// Localization: built-in Russian and English plus languages uploaded as JSON.
// Messages are grouped by page and UI element; a key is "section.key" or
// "section.group.key". A missing key falls back to English, then Russian,
// then to the key itself, so a partly filled file still works.

import { sections } from './locales'

export type Msg = string | { [k: string]: Msg }
export type Dict = { [k: string]: Msg }

export interface Locale {
  id: string // BCP 47 tag, e.g. "ru", "en", "de"
  name: string // shown in the language list, e.g. "Deutsch"
  dateLocale: string // used for dates and numbers, e.g. "de-DE"
  builtIn?: boolean
  messages: Dict
}

function build(lang: 'ru' | 'en'): Dict {
  const out: Dict = {}
  for (const [name, s] of Object.entries(sections)) out[name] = s[lang]
  return out
}

export const BUILT_IN_LOCALES: Locale[] = [
  { id: 'ru', name: 'Русский', dateLocale: 'ru-RU', builtIn: true, messages: build('ru') },
  { id: 'en', name: 'English', dateLocale: 'en-GB', builtIn: true, messages: build('en') },
]

const STORE_KEY = 'umb.locales'
const ACTIVE_KEY = 'umb.locale'

export function loadCustomLocales(): Locale[] {
  try {
    const raw = localStorage.getItem(STORE_KEY)
    if (!raw) return []
    const list = JSON.parse(raw) as Locale[]
    return Array.isArray(list) ? list.filter((l) => l && typeof l.id === 'string' && l.messages) : []
  } catch {
    return []
  }
}

export function saveCustomLocales(list: Locale[]) {
  try {
    localStorage.setItem(STORE_KEY, JSON.stringify(list))
  } catch {
    /* storage unavailable */
  }
}

export function loadActiveLocale(): string {
  try {
    const v = localStorage.getItem(ACTIVE_KEY)
    if (v) return v
  } catch {
    /* ignore */
  }
  return 'ru'
}

export function saveActiveLocale(id: string) {
  try {
    localStorage.setItem(ACTIVE_KEY, id)
  } catch {
    /* ignore */
  }
}

function lookup(d: Dict, key: string): Msg | undefined {
  let cur: Msg | undefined = d
  for (const part of key.split('.')) {
    if (!cur || typeof cur === 'string') return undefined
    cur = cur[part]
  }
  return cur
}

export type Vars = Record<string, string | number | undefined>

// Plural forms are objects with CLDR categories: {"one": "...", "few": "...",
// "many": "...", "other": "..."}; the form is picked by vars.n.
function pick(m: Msg | undefined, vars: Vars | undefined, lang: string): string | undefined {
  if (m === undefined) return undefined
  if (typeof m === 'string') return m
  if (typeof m.other === 'string') {
    const n = Number(vars?.n ?? 0)
    let cat = 'other'
    try {
      cat = new Intl.PluralRules(lang).select(n)
    } catch {
      /* unknown tag */
    }
    const v = m[cat] ?? m.other
    return typeof v === 'string' ? v : undefined
  }
  return undefined
}

export type T = (key: string, vars?: Vars) => string

export function makeT(active: Locale): T {
  const chain = [active, BUILT_IN_LOCALES[1], BUILT_IN_LOCALES[0]]
  return (key, vars) => {
    let s: string | undefined
    for (const l of chain) {
      s = pick(lookup(l.messages, key), vars, l.id)
      if (s !== undefined) break
    }
    if (s === undefined) return key
    if (!vars) return s
    return s.replace(/\{(\w+)\}/g, (all, k) => (vars[k] === undefined ? all : String(vars[k])))
  }
}

// ---- template download and upload ----

export function localeTemplate(base: Locale): object {
  return {
    $schema: 'umbrella-locale/v1',
    $help:
      'Translate every string in "messages". Keep keys and {placeholders} as they are. Plural forms use CLDR categories (one, few, many, other). Set "id" to a language tag (de, kk, uz-Latn) and "dateLocale" to a locale for dates (de-DE). Missing strings fall back to English.',
    id: 'xx',
    name: 'Language name',
    dateLocale: 'xx-XX',
    messages: base.messages,
  }
}

export function parseLocale(text: string): Locale {
  let data: unknown
  try {
    data = JSON.parse(text)
  } catch (e) {
    throw new Error(`JSON: ${(e as Error).message}`)
  }
  const o = data as Partial<Locale>
  if (!o || typeof o !== 'object') throw new Error('root must be an object')
  if (typeof o.id !== 'string' || !/^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$/.test(o.id) || o.id === 'xx') throw new Error('id: a language tag is required, e.g. "de"')
  if (typeof o.name !== 'string' || !o.name.trim() || o.name === 'Language name') throw new Error('name is required')
  if (!o.messages || typeof o.messages !== 'object') throw new Error('messages must be an object')
  const bad = findNonStrings(o.messages as Dict, '')
  if (bad) throw new Error(`messages.${bad}: value must be a string or an object`)
  let dateLocale = typeof o.dateLocale === 'string' && o.dateLocale !== 'xx-XX' ? o.dateLocale : o.id
  try {
    new Intl.DateTimeFormat(dateLocale)
  } catch {
    dateLocale = 'en-GB'
  }
  return { id: o.id, name: o.name.trim(), dateLocale, messages: o.messages as Dict }
}

function findNonStrings(d: Dict, path: string): string | null {
  for (const [k, v] of Object.entries(d)) {
    const p = path ? `${path}.${k}` : k
    if (typeof v === 'string') continue
    if (v && typeof v === 'object' && !Array.isArray(v)) {
      const r = findNonStrings(v, p)
      if (r) return r
      continue
    }
    return p
  }
  return null
}

// countKeys returns how many strings a dictionary has and how many of the
// reference keys it covers, for the coverage figure in settings.
export function coverage(d: Dict, ref: Dict): { total: number; covered: number } {
  let total = 0
  let covered = 0
  const walk = (r: Dict, x: Msg | undefined) => {
    for (const [k, v] of Object.entries(r)) {
      const xv = x && typeof x === 'object' ? x[k] : undefined
      if (typeof v === 'string') {
        total++
        if (typeof xv === 'string') covered++
      } else if (typeof v.other === 'string') {
        total++
        if (xv && typeof xv === 'object' && typeof xv.other === 'string') covered++
      } else walk(v, xv)
    }
  }
  walk(ref, d)
  return { total, covered }
}

export function download(name: string, data: unknown) {
  const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = name
  document.body.appendChild(a)
  a.click()
  a.remove()
  window.setTimeout(() => URL.revokeObjectURL(a.href), 1000)
}

// ---- active language ----
// The provider sets the active language before rendering and remounts the
// tree when it changes, so plain functions (t, fmtTime) can read it directly.

let activeT: T = makeT(BUILT_IN_LOCALES[0])
let activeDate = BUILT_IN_LOCALES[0].dateLocale

export function setActiveLocale(l: Locale) {
  activeT = makeT(l)
  activeDate = l.dateLocale
  document.documentElement.lang = l.id
}

export const t: T = (key, vars) => activeT(key, vars)
export const dateLocale = () => activeDate
