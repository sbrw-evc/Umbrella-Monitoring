// Motion preference and animated switches (theme, language).
//
// "auto" follows the system prefers-reduced-motion setting, "on" and "off"
// override it. The value lives in html[data-motion], which styles.css reads.

export type Motion = 'auto' | 'on' | 'off'

const KEY = 'umb.motion'

export function loadMotion(): Motion {
  try {
    const v = localStorage.getItem(KEY)
    return v === 'on' || v === 'off' ? v : 'auto'
  } catch {
    return 'auto'
  }
}

export function applyMotion(m: Motion) {
  if (m === 'auto') delete document.documentElement.dataset.motion
  else document.documentElement.dataset.motion = m
}

export function saveMotion(m: Motion) {
  applyMotion(m)
  try {
    localStorage.setItem(KEY, m)
  } catch {
    /* storage unavailable */
  }
}

export function motionOn(): boolean {
  const m = document.documentElement.dataset.motion
  if (m === 'off') return false
  if (m === 'on') return true
  return !window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
}

// The last pointer press: the theme reveal starts there.
let px = window.innerWidth - 120
let py = 28
window.addEventListener('pointerdown', (e) => {
  px = e.clientX
  py = e.clientY
})

type VT = { finished: Promise<void> }
type DocVT = Document & { startViewTransition?: (cb: () => void) => VT }

// transition runs update inside a view transition when the browser has one
// and motion is on; otherwise it just runs it (themes then fade colors).
export function transition(kind: 'theme' | 'locale', update: () => void) {
  const doc = document as DocVT
  const root = document.documentElement
  if (!motionOn()) {
    update()
    return
  }
  if (!doc.startViewTransition) {
    if (kind === 'theme') {
      root.classList.add('theme-fading')
      window.setTimeout(() => root.classList.remove('theme-fading'), 400)
    }
    update()
    return
  }
  const cls = `vt-${kind}`
  root.classList.add(cls)
  if (kind === 'theme') {
    const r = Math.hypot(Math.max(px, window.innerWidth - px), Math.max(py, window.innerHeight - py))
    root.style.setProperty('--vt-x', `${px}px`)
    root.style.setProperty('--vt-y', `${py}px`)
    root.style.setProperty('--vt-r', `${r}px`)
  }
  doc.startViewTransition(update).finished.finally(() => root.classList.remove(cls))
}
