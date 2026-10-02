// Color themes. Every color the UI uses is a CSS variable named
// --<element>-<token> (for example --table-row-hover). A theme is a JSON
// object with the same grouping, so a downloaded template shows exactly which
// element each color paints. Missing tokens are taken from the base theme.

export type ThemeColors = Record<string, Record<string, string>>

export interface Theme {
  id: string
  name: string
  base: 'light' | 'dark'
  builtIn?: boolean
  colors: ThemeColors
}

const LIGHT: ThemeColors = {
  app: {
    background: '#f4f6f9',
    surface: '#ffffff',
    surfaceAlt: '#f7f8fa',
    text: '#1d2530',
    textMuted: '#6b7685',
    textSoft: '#465260',
    link: '#2563eb',
    border: '#e4e8ee',
    borderSoft: '#eef1f5',
    focusRing: 'rgba(37, 99, 235, 0.22)',
    scrollbar: '#cdd4dd',
  },
  accent: {
    primary: '#2563eb',
    primaryHover: '#1d4fd8',
    primaryText: '#ffffff',
    soft: '#eaf1ff',
    softText: '#1a46b8',
  },
  rail: {
    background: '#ffffff',
    border: '#e8ebf0',
    icon: '#6b7685',
    iconActive: '#2563eb',
    text: '#33404e',
    groupTitle: '#98a2b0',
    itemHover: '#f2f4f7',
    itemActive: '#eaf1ff',
    logoBackground: '#1d2530',
    logoMark: '#4f8bff',
  },
  topbar: {
    background: '#ffffff',
    border: '#e8ebf0',
    text: '#1d2530',
    searchBackground: '#f2f4f7',
  },
  sideList: {
    background: '#ffffff',
    border: '#e8ebf0',
    itemHover: '#f4f6f9',
    itemActive: '#eaf1ff',
    itemActiveText: '#1a46b8',
    countBackground: '#eef1f5',
    countText: '#5b6675',
  },
  button: {
    background: '#ffffff',
    border: '#dce1e8',
    text: '#1d2530',
    hover: '#f4f6f9',
    dangerBackground: '#e03e47',
    dangerHover: '#c9343c',
    dangerText: '#ffffff',
  },
  input: {
    background: '#ffffff',
    border: '#dce1e8',
    text: '#1d2530',
    placeholder: '#98a2b0',
  },
  table: {
    headerBackground: '#ffffff',
    headerText: '#6b7685',
    rowBorder: '#eef1f5',
    rowHover: '#f7f9fc',
    rowSelected: '#eef4ff',
    detailBackground: '#f7f8fa',
  },
  severity: {
    critical: '#e03e47',
    error: '#ef6c2a',
    warning: '#e9a812',
    info: '#3b82f6',
    ok: '#1f9d62',
    criticalSoft: '#fdecee',
    errorSoft: '#fef0e7',
    warningSoft: '#fdf5df',
    infoSoft: '#eaf2fe',
    okSoft: '#e5f5ec',
    criticalText: '#b4232c',
    errorText: '#b14812',
    warningText: '#8a6200',
    infoText: '#1f5fc7',
    okText: '#16774a',
  },
  status: {
    openBackground: '#fdecee',
    openText: '#b4232c',
    acknowledgedBackground: '#fdf5df',
    acknowledgedText: '#8a6200',
    resolvedBackground: '#e5f5ec',
    resolvedText: '#16774a',
    runningBackground: '#e5f5ec',
    runningText: '#16774a',
    mutedBackground: '#eef1f5',
    mutedText: '#5b6675',
  },
  pagerduty: {
    acceptedText: '#16774a',
    ackedText: '#8a6200',
    failedBackground: '#e03e47',
    failedText: '#ffffff',
    fallbackBackground: '#4338ca',
    fallbackText: '#ffffff',
  },
  tag: {
    background: '#eef1f5',
    text: '#465260',
    linkBackground: '#eaf1ff',
    linkText: '#1a46b8',
  },
  graph: {
    canvas: '#f7f8fa',
    grid: '#e1e6ec',
    edge: '#a3adba',
    nodeBackground: '#ffffff',
    nodeBorder: '#e1e6ec',
    selection: '#2563eb',
  },
  blocks: {
    trigger: '#7c5cff',
    fetch: '#2f80ed',
    parse: '#0d9e9e',
    transform: '#ef8a2a',
    ack: '#1f9d62',
    output: '#c9970a',
  },
  chart: {
    emptyBar: '#eef1f5',
  },
  overlay: {
    backdrop: 'rgba(17, 24, 39, 0.28)',
    popupBackground: '#ffffff',
    shadow: 'rgba(17, 24, 39, 0.10)',
  },
  toast: {
    background: '#1d2530',
    text: '#ffffff',
  },
  code: {
    background: '#f6f8fa',
    text: '#33404e',
  },
}

const DARK: ThemeColors = {
  app: {
    background: '#0f1318',
    surface: '#171c23',
    surfaceAlt: '#1d232c',
    text: '#e5e9ef',
    textMuted: '#8e99a8',
    textSoft: '#b5bfcb',
    link: '#79a8ff',
    border: '#2a323d',
    borderSoft: '#222a34',
    focusRing: 'rgba(96, 145, 255, 0.35)',
    scrollbar: '#38424f',
  },
  accent: {
    primary: '#4d84ff',
    primaryHover: '#6a98ff',
    primaryText: '#ffffff',
    soft: '#1b2a45',
    softText: '#a8c4ff',
  },
  rail: {
    background: '#141920',
    border: '#232a34',
    icon: '#8e99a8',
    iconActive: '#79a8ff',
    text: '#c9d1db',
    groupTitle: '#667180',
    itemHover: '#1d232c',
    itemActive: '#1b2a45',
    logoBackground: '#242c37',
    logoMark: '#79a8ff',
  },
  topbar: {
    background: '#141920',
    border: '#232a34',
    text: '#e5e9ef',
    searchBackground: '#1d232c',
  },
  sideList: {
    background: '#141920',
    border: '#232a34',
    itemHover: '#1d232c',
    itemActive: '#1b2a45',
    itemActiveText: '#a8c4ff',
    countBackground: '#242c37',
    countText: '#b5bfcb',
  },
  button: {
    background: '#1d232c',
    border: '#313a46',
    text: '#e5e9ef',
    hover: '#252d38',
    dangerBackground: '#d9434b',
    dangerHover: '#e85a61',
    dangerText: '#ffffff',
  },
  input: {
    background: '#12171d',
    border: '#313a46',
    text: '#e5e9ef',
    placeholder: '#667180',
  },
  table: {
    headerBackground: '#171c23',
    headerText: '#8e99a8',
    rowBorder: '#222a34',
    rowHover: '#1c222b',
    rowSelected: '#1a2640',
    detailBackground: '#12171d',
  },
  severity: {
    critical: '#f0525b',
    error: '#f57c3d',
    warning: '#f2b632',
    info: '#5b9bff',
    ok: '#36b47a',
    criticalSoft: '#3a1c21',
    errorSoft: '#3a2418',
    warningSoft: '#372c14',
    infoSoft: '#18294a',
    okSoft: '#15301f',
    criticalText: '#ff9aa0',
    errorText: '#ffb185',
    warningText: '#f7cf72',
    infoText: '#a3c6ff',
    okText: '#7fd9a9',
  },
  status: {
    openBackground: '#3a1c21',
    openText: '#ff9aa0',
    acknowledgedBackground: '#372c14',
    acknowledgedText: '#f7cf72',
    resolvedBackground: '#15301f',
    resolvedText: '#7fd9a9',
    runningBackground: '#15301f',
    runningText: '#7fd9a9',
    mutedBackground: '#242c37',
    mutedText: '#b5bfcb',
  },
  pagerduty: {
    acceptedText: '#7fd9a9',
    ackedText: '#f7cf72',
    failedBackground: '#d9434b',
    failedText: '#ffffff',
    fallbackBackground: '#5b54e8',
    fallbackText: '#ffffff',
  },
  tag: {
    background: '#242c37',
    text: '#c9d1db',
    linkBackground: '#1b2a45',
    linkText: '#a8c4ff',
  },
  graph: {
    canvas: '#12171d',
    grid: '#232a34',
    edge: '#5d6876',
    nodeBackground: '#1d232c',
    nodeBorder: '#313a46',
    selection: '#79a8ff',
  },
  blocks: {
    trigger: '#9b84ff',
    fetch: '#5b9bff',
    parse: '#2cc4c4',
    transform: '#f59e4b',
    ack: '#36b47a',
    output: '#e0b23a',
  },
  chart: {
    emptyBar: '#222a34',
  },
  overlay: {
    backdrop: 'rgba(0, 0, 0, 0.5)',
    popupBackground: '#1d232c',
    shadow: 'rgba(0, 0, 0, 0.45)',
  },
  toast: {
    background: '#e5e9ef',
    text: '#0f1318',
  },
  code: {
    background: '#12171d',
    text: '#c9d1db',
  },
}

export const BUILT_IN_THEMES: Theme[] = [
  { id: 'light', name: 'light', base: 'light', builtIn: true, colors: LIGHT },
  { id: 'dark', name: 'dark', base: 'dark', builtIn: true, colors: DARK },
]

// What each element group paints, for the template's "$help" block.
export const THEME_GROUP_HELP: Record<string, string> = {
  app: 'Page background, cards and panels, main and secondary text, links, borders, focus ring',
  accent: 'Primary buttons, active tab underline, selected items, focus',
  rail: 'Left navigation rail: background, icons, active and hovered items, logo',
  topbar: 'Top bar with search, scope, language and theme switches',
  sideList: 'Secondary list panel left of a page: saved views, CI list',
  button: 'Regular and danger buttons',
  input: 'Text inputs, selects, text areas',
  table: 'Table header, row separators, hovered and selected rows',
  severity: 'Severity colors (solid for dots and bars, Soft for chip backgrounds, Text for chip text)',
  status: 'Incident status chips and connector running state',
  pagerduty: 'PagerDuty delivery state chips and fallback marker',
  tag: 'Small tags (labels, methods, links)',
  graph: 'CMDB graph and connector canvas: background, grid, edges, nodes',
  blocks: 'Connector builder block categories',
  chart: 'Charts: empty bar background',
  overlay: 'Drawer and modal backdrop, popup menus, shadows',
  toast: 'Notifications in the corner',
  code: 'JSON and raw payload boxes',
}

const kebab = (s: string) => s.replace(/[A-Z]/g, (c) => '-' + c.toLowerCase())

export function cssVarName(group: string, token: string) {
  return `--${kebab(group)}-${kebab(token)}`
}

export function resolveColors(t: Theme): ThemeColors {
  const base = t.base === 'dark' ? DARK : LIGHT
  const out: ThemeColors = {}
  for (const [g, tokens] of Object.entries(base)) out[g] = { ...tokens, ...(t.colors[g] ?? {}) }
  return out
}

export function applyTheme(t: Theme) {
  const root = document.documentElement
  const colors = resolveColors(t)
  for (const [g, tokens] of Object.entries(colors)) {
    for (const [k, v] of Object.entries(tokens)) root.style.setProperty(cssVarName(g, k), v)
  }
  root.dataset.theme = t.base
  root.style.colorScheme = t.base
}

const STORE_KEY = 'umb.themes'
const ACTIVE_KEY = 'umb.theme'

export function loadCustomThemes(): Theme[] {
  try {
    const raw = localStorage.getItem(STORE_KEY)
    const list = raw ? (JSON.parse(raw) as Theme[]) : []
    return Array.isArray(list) ? list.filter((t) => t && typeof t.id === 'string' && t.colors) : []
  } catch {
    return []
  }
}

export function saveCustomThemes(list: Theme[]) {
  try {
    localStorage.setItem(STORE_KEY, JSON.stringify(list))
  } catch {
    /* storage unavailable */
  }
}

export function loadActiveTheme(): string {
  try {
    const v = localStorage.getItem(ACTIVE_KEY)
    if (v) return v
  } catch {
    /* ignore */
  }
  return window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

export function saveActiveTheme(id: string) {
  try {
    localStorage.setItem(ACTIVE_KEY, id)
  } catch {
    /* ignore */
  }
}

export function themeTemplate(base: Theme): object {
  return {
    $schema: 'umbrella-theme/v1',
    $help: {
      about:
        'Fill in the colors and upload the file in Settings > Themes. Any CSS color works: #rrggbb, rgb(), rgba(), hsl(). "base" (light or dark) supplies every color you leave out.',
      groups: THEME_GROUP_HELP,
    },
    id: 'my-theme',
    name: 'My theme',
    base: base.base,
    colors: resolveColors(base),
  }
}

function validColor(v: string): boolean {
  if (typeof CSS !== 'undefined' && CSS.supports) return CSS.supports('color', v)
  return /^(#[0-9a-f]{3,8}|(rgb|hsl)a?\(.+\))$/i.test(v.trim())
}

export function parseTheme(text: string): Theme {
  let data: unknown
  try {
    data = JSON.parse(text)
  } catch (e) {
    throw new Error(`JSON: ${(e as Error).message}`)
  }
  const o = data as Partial<Theme>
  if (!o || typeof o !== 'object') throw new Error('root must be an object')
  if (typeof o.id !== 'string' || !/^[a-z0-9][a-z0-9-]{1,40}$/.test(o.id)) throw new Error('id: lowercase letters, digits and dashes, e.g. "corporate-blue"')
  if (o.id === 'light' || o.id === 'dark') throw new Error('id: "light" and "dark" are built in, pick another id')
  if (typeof o.name !== 'string' || !o.name.trim()) throw new Error('name is required')
  const base = o.base === 'dark' ? 'dark' : 'light'
  if (!o.colors || typeof o.colors !== 'object') throw new Error('colors must be an object')
  const ref = base === 'dark' ? DARK : LIGHT
  const colors: ThemeColors = {}
  for (const [g, tokens] of Object.entries(o.colors)) {
    if (!ref[g]) throw new Error(`colors.${g}: unknown element`)
    if (!tokens || typeof tokens !== 'object') throw new Error(`colors.${g} must be an object`)
    colors[g] = {}
    for (const [k, v] of Object.entries(tokens as Record<string, unknown>)) {
      if (!(k in ref[g])) throw new Error(`colors.${g}.${k}: unknown color`)
      if (typeof v !== 'string' || !validColor(v)) throw new Error(`colors.${g}.${k}: "${String(v)}" is not a color`)
      colors[g][k] = v
    }
  }
  return { id: o.id, name: o.name.trim(), base, colors }
}
