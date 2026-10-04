import { useId, useState, type ButtonHTMLAttributes, type InputHTMLAttributes, type ReactNode, type SelectHTMLAttributes, type TextareaHTMLAttributes } from 'react'
import { AlertTriangle, CheckCircle2, Eye, EyeOff, Info, Loader2, Moon, Sun, XCircle } from 'lucide-react'
import { useLocale, useT } from './i18n'
import { useTheme } from './theme'

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & { variant?: 'primary' | 'secondary' | 'ghost'; busy?: boolean }

export function Button({ variant = 'secondary', busy, disabled, children, className, ...rest }: ButtonProps) {
  return (
    <button {...rest} className={`btn btn-${variant} ${className ?? ''}`} disabled={disabled || busy} aria-busy={busy || undefined}>
      {busy && <Loader2 className="spin" size={16} aria-hidden />}
      {children}
    </button>
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

export function Segmented<T extends string>({ value, options, onChange, label }: { value: T; options: { value: T; label: ReactNode }[]; onChange: (v: T) => void; label: string }) {
  return (
    <div className="segmented" role="radiogroup" aria-label={label}>
      {options.map((o) => (
        <button key={o.value} type="button" role="radio" aria-checked={value === o.value} className={value === o.value ? 'active' : ''} onClick={() => onChange(o.value)}>
          {o.label}
        </button>
      ))}
    </div>
  )
}

const icons = { ok: CheckCircle2, error: XCircle, warn: AlertTriangle, info: Info }

export function Banner({ kind, title, children }: { kind: keyof typeof icons; title: ReactNode; children?: ReactNode }) {
  const Icon = icons[kind]
  return (
    <div className={`banner banner-${kind}`} role={kind === 'error' ? 'alert' : 'status'}>
      <Icon size={18} aria-hidden />
      <div>
        <div className="banner-title">{title}</div>
        {children && <div className="banner-body">{children}</div>}
      </div>
    </div>
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
      <img src="/umbrella.svg" alt="" width={28} height={28} />
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
      <button type="button" className="icon-btn" aria-label={t('theme.toggle')} title={t('theme.toggle')} onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark', persist)}>
        {theme === 'dark' ? <Sun size={18} /> : <Moon size={18} />}
      </button>
    </div>
  )
}

export function formatDate(v: string | undefined | null, locale: string) {
  if (!v) return ''
  const d = new Date(v)
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 2000) return ''
  return d.toLocaleString(locale === 'ru' ? 'ru-RU' : 'en-GB')
}

const labels = {
  en: { 'ui.show': 'Show', 'ui.hide': 'Hide' },
  ru: { 'ui.show': 'Показать', 'ui.hide': 'Скрыть' },
}
