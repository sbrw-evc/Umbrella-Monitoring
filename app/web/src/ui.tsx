import {
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type InputHTMLAttributes,
  type ReactNode,
  type SelectHTMLAttributes,
  type TextareaHTMLAttributes,
} from 'react'
import { AlertTriangle, CheckCircle2, Eye, EyeOff, Info, Loader2, Moon, Sun, X, XCircle } from 'lucide-react'
import { AnimatePresence, motion, type HTMLMotionProps, type Transition } from 'motion/react'
import { useLocale, useT } from './i18n'
import { useTheme } from './theme'
import { originOf } from './fx'

export const spring: Transition = { type: 'spring', stiffness: 420, damping: 30 }

type ButtonProps = Omit<HTMLMotionProps<'button'>, 'children'> & { children?: ReactNode; variant?: 'primary' | 'secondary' | 'ghost'; busy?: boolean }

export function Button({ variant = 'secondary', busy, disabled, children, className, ...rest }: ButtonProps) {
  return (
    <motion.button
      {...rest}
      className={`btn btn-${variant} ${className ?? ''}`}
      disabled={disabled || busy}
      aria-busy={busy || undefined}
      whileTap={disabled || busy ? undefined : { scale: 0.97 }}
      transition={spring}
    >
      {busy && <Loader2 className="spin" size={16} aria-hidden />}
      {children}
    </motion.button>
  )
}

export function Field({ label, hint, children, optional }: { label: string; hint?: ReactNode; children: (id: string) => ReactNode; optional?: string }) {
  const id = useId()
  return (
    <div className="field">
      <label htmlFor={id}>
        {label}
        {optional && <span className="optional"> ({optional})</span>}
      </label>
      {children(id)}
      {hint && <div className="hint">{hint}</div>}
    </div>
  )
}

export function Input(props: InputHTMLAttributes<HTMLInputElement>) {
  return <input {...props} className={`input ${props.className ?? ''}`} />
}

export function Password(props: InputHTMLAttributes<HTMLInputElement>) {
  const [shown, setShown] = useState(false)
  const t = useT(labels)
  return (
    <div className="password">
      <input {...props} type={shown ? 'text' : 'password'} className="input" />
      <button type="button" className="icon-btn" onClick={() => setShown(!shown)} aria-label={shown ? t('ui.hide') : t('ui.show')}>
        {shown ? <EyeOff size={16} /> : <Eye size={16} />}
      </button>
    </div>
  )
}

export function Select(props: SelectHTMLAttributes<HTMLSelectElement>) {
  return <select {...props} className="input" />
}

export function Textarea(props: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return <textarea {...props} className="input textarea" />
}

export function Switch({ checked, onChange, label, hint }: { checked: boolean; onChange: (v: boolean) => void; label: string; hint?: string }) {
  const id = useId()
  return (
    <div className="switch-row">
      <button id={id} type="button" role="switch" aria-checked={checked} className={`switch ${checked ? 'on' : ''}`} onClick={() => onChange(!checked)}>
        <span />
      </button>
      <label htmlFor={id}>
        <span>{label}</span>
        {hint && <span className="hint">{hint}</span>}
      </label>
    </div>
  )
}

export function Segmented<T extends string>({
  value,
  options,
  onChange,
  label,
}: {
  value: T
  options: { value: T; label: ReactNode }[]
  onChange: (v: T) => void
  label: string
}) {
  const group = useId()
  return (
    <div className="segmented" role="radiogroup" aria-label={label}>
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          role="radio"
          aria-checked={value === o.value}
          className={value === o.value ? 'active' : ''}
          onClick={() => onChange(o.value)}
        >
          {value === o.value && <motion.span layoutId={`seg-${group}`} className="seg-pill" transition={spring} />}
          <span className="seg-label">{o.label}</span>
        </button>
      ))}
    </div>
  )
}

const icons = { ok: CheckCircle2, error: XCircle, warn: AlertTriangle, info: Info }

export function Banner({ kind, title, children }: { kind: keyof typeof icons; title: ReactNode; children?: ReactNode }) {
  const Icon = icons[kind]
  return (
    <motion.div
      className={`banner banner-${kind}`}
      role={kind === 'error' ? 'alert' : 'status'}
      initial={{ opacity: 0, y: 8 }}
      animate={kind === 'error' ? { opacity: 1, y: 0, x: [0, -6, 5, -3, 0] } : { opacity: 1, y: 0 }}
      transition={{ duration: 0.35, ease: [0.22, 1, 0.36, 1] }}
    >
      <Icon size={18} aria-hidden />
      <div>
        <div className="banner-title">{title}</div>
        {children && <div className="banner-body">{children}</div>}
      </div>
    </motion.div>
  )
}

export function Rows({ rows }: { rows: [ReactNode, ReactNode][] }) {
  return (
    <dl className="rows">
      {rows.map(([k, v], i) => (
        <div key={i}>
          <dt>{k}</dt>
          <dd>{v === '' || v === undefined || v === null ? '—' : v}</dd>
        </div>
      ))}
    </dl>
  )
}

export function Brand({ subtitle }: { subtitle?: string }) {
  const t = useT()
  return (
    <div className="brand">
      <img src="/logo.svg" alt="" width={32} height={32} />
      <div>
        <div className="brand-name">{t('app.name')}</div>
        {subtitle && <div className="brand-sub">{subtitle}</div>}
      </div>
    </div>
  )
}

export function Preferences({ persist = true }: { persist?: boolean }) {
  const { theme, setTheme } = useTheme()
  const { locale, setLocale } = useLocale()
  const t = useT()
  return (
    <div className="prefs">
      <Segmented
        label={t('lang.toggle')}
        value={locale}
        onChange={(l) => setLocale(l, persist)}
        options={[
          { value: 'en', label: 'EN' },
          { value: 'ru', label: 'RU' },
        ]}
      />
      <button
        type="button"
        className="icon-btn"
        aria-label={t('theme.toggle')}
        title={t('theme.toggle')}
        onClick={(e) => setTheme(theme === 'dark' ? 'light' : 'dark', persist, originOf(e))}
      >
        <AnimatePresence mode="wait" initial={false}>
          <motion.span
            key={theme}
            className="pop-icon"
            initial={{ rotate: -90, scale: 0.4, opacity: 0 }}
            animate={{ rotate: 0, scale: 1, opacity: 1 }}
            exit={{ rotate: 90, scale: 0.4, opacity: 0 }}
            transition={{ duration: 0.22 }}
          >
            {theme === 'dark' ? <Sun size={18} /> : <Moon size={18} />}
          </motion.span>
        </AnimatePresence>
      </button>
    </div>
  )
}

export function formatDate(v: string | undefined | null, locale: string, timeZone?: string) {
  if (!v) return ''
  const d = new Date(v)
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 2000) return ''
  try {
    return d.toLocaleString(locale === 'ru' ? 'ru-RU' : 'en-GB', timeZone ? { timeZone } : undefined)
  } catch {
    return d.toLocaleString(locale === 'ru' ? 'ru-RU' : 'en-GB')
  }
}

export function browserTimezone() {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
  } catch {
    return 'UTC'
  }
}

function offsetMinutes(zone: string, at: Date) {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone: zone,
    hourCycle: 'h23',
    year: 'numeric',
    month: 'numeric',
    day: 'numeric',
    hour: 'numeric',
    minute: 'numeric',
  })
    .formatToParts(at)
    .reduce<Record<string, number>>((acc, p) => (p.type === 'literal' ? acc : { ...acc, [p.type]: Number(p.value) }), {})
  const asUTC = Date.UTC(parts.year, parts.month - 1, parts.day, parts.hour, parts.minute)
  return Math.round((asUTC - Math.floor(at.getTime() / 60000) * 60000) / 60000)
}

function offsetLabel(min: number) {
  const sign = min < 0 ? '−' : '+'
  const a = Math.abs(min)
  return `UTC${sign}${String(Math.floor(a / 60)).padStart(2, '0')}:${String(a % 60).padStart(2, '0')}`
}

export type Zone = { id: string; offset: number; label: string }

let zoneCache: Zone[] | null = null

export function timezones(): Zone[] {
  if (zoneCache) return zoneCache
  const now = new Date()
  let ids: string[] = []
  try {
    ids = Intl.supportedValuesOf('timeZone')
  } catch {
    ids = []
  }
  if (!ids.includes('UTC')) ids = ['UTC', ...ids]
  zoneCache = ids
    .map((id) => {
      const offset = offsetMinutes(id, now)
      return { id, offset, label: `(${offsetLabel(offset)}) ${id.replaceAll('_', ' ')}` }
    })
    .sort((a, b) => a.offset - b.offset || a.id.localeCompare(b.id))
  return zoneCache
}

export function zoneLabel(id: string) {
  return timezones().find((z) => z.id === id)?.label ?? id
}

export function TimezoneSelect({ value, onChange, id, defaultLabel }: { value: string; onChange: (v: string) => void; id?: string; defaultLabel?: string }) {
  const zones = useMemo(timezones, [])
  const known = value === '' || zones.some((z) => z.id === value)
  return (
    <select id={id} className="input" value={value} onChange={(e) => onChange(e.target.value)}>
      {defaultLabel !== undefined && <option value="">{defaultLabel}</option>}
      {!known && <option value={value}>{value}</option>}
      {zones.map((z) => (
        <option key={z.id} value={z.id}>
          {z.label}
        </option>
      ))}
    </select>
  )
}

export function Modal({
  open,
  title,
  onClose,
  children,
  footer,
}: {
  open: boolean
  title: string
  onClose: () => void
  children: ReactNode
  footer?: ReactNode
}) {
  const t = useT(labels)
  const box = useRef<HTMLDivElement>(null)
  const titleId = useId()
  useEffect(() => {
    if (!open) return
    const prev = document.activeElement as HTMLElement | null
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose()
    window.addEventListener('keydown', onKey)
    requestAnimationFrame(() => box.current?.querySelector<HTMLElement>('input, select, textarea, button')?.focus())
    return () => {
      window.removeEventListener('keydown', onKey)
      prev?.focus()
    }
  }, [open, onClose])
  return (
    <AnimatePresence>
      {open && (
        <motion.div
          className="modal-backdrop"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          onMouseDown={(e) => e.target === e.currentTarget && onClose()}
        >
          <motion.div
            ref={box}
            className="card modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby={titleId}
            initial={{ opacity: 0, y: 16, scale: 0.97 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, y: 8, scale: 0.98 }}
            transition={spring}
          >
            <header className="modal-head">
              <h2 id={titleId}>{title}</h2>
              <button type="button" className="icon-btn" onClick={onClose} aria-label={t('ui.close')}>
                <X size={18} />
              </button>
            </header>
            <div className="stack">{children}</div>
            {footer && <div className="modal-foot">{footer}</div>}
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
  )
}

const labels = {
  en: { 'ui.show': 'Show', 'ui.hide': 'Hide', 'ui.close': 'Close' },
  ru: { 'ui.show': 'Показать', 'ui.hide': 'Скрыть', 'ui.close': 'Закрыть' },
}
