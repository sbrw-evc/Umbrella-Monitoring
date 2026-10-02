import { X } from 'lucide-react'
import { type ReactNode, useEffect } from 'react'
import { PD_LABEL, SEV_LABEL, STATUS_LABEL, type AlertStatus, type PDState, type Severity } from '../api'

export function SevBadge({ sev }: { sev: Severity | '' }) {
  if (!sev) return <span className="sev sev-ok">OK</span>
  return <span className={`sev sev-${sev}`}>{SEV_LABEL[sev]}</span>
}

export function SevDot({ sev, title }: { sev: Severity | ''; title?: string }) {
  return <span className={`dot dot-${sev || 'ok'}`} title={title ?? (sev ? SEV_LABEL[sev] : 'OK')} />
}

export function StatusPill({ status }: { status: AlertStatus }) {
  return <span className={`pill pill-${status}`}>{STATUS_LABEL[status]}</span>
}

export function PDPill({ state, fallback }: { state: PDState; fallback?: boolean }) {
  return (
    <span className="pd-cell">
      <span className={`pd pd-${state}`}>{PD_LABEL[state]}</span>
      {fallback && (
        <span className="pd pd-fallback" title="Отправлено по резервным каналам">
          Резерв
        </span>
      )}
    </span>
  )
}

export function PageHeader({ title, sub, actions }: { title: string; sub?: ReactNode; actions?: ReactNode }) {
  return (
    <div className="page-header">
      <div>
        <h1>{title}</h1>
        {sub && <div className="page-sub">{sub}</div>}
      </div>
      <div className="page-actions">{actions}</div>
    </div>
  )
}

export function Tabs<T extends string>({
  tabs,
  value,
  onChange,
}: {
  tabs: { id: T; title: ReactNode }[]
  value: T
  onChange: (v: T) => void
}) {
  return (
    <div className="tabs">
      {tabs.map((t) => (
        <button key={t.id} className={`tab ${value === t.id ? 'tab-active' : ''}`} onClick={() => onChange(t.id)}>
          {t.title}
        </button>
      ))}
    </div>
  )
}

export function Drawer({
  open,
  onClose,
  title,
  sub,
  actions,
  children,
  wide,
}: {
  open: boolean
  onClose: () => void
  title: ReactNode
  sub?: ReactNode
  actions?: ReactNode
  children: ReactNode
  wide?: boolean
}) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose()
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])
  if (!open) return null
  return (
    <div className="drawer-wrap" onMouseDown={onClose}>
      <aside className={`drawer ${wide ? 'drawer-wide' : ''}`} onMouseDown={(e) => e.stopPropagation()}>
        <div className="drawer-head">
          <div className="drawer-title">
            {title}
            {sub && <div className="drawer-sub">{sub}</div>}
          </div>
          <div className="drawer-actions">
            {actions}
            <button className="icon-btn" onClick={onClose} title="Закрыть">
              <X size={18} />
            </button>
          </div>
        </div>
        <div className="drawer-body">{children}</div>
      </aside>
    </div>
  )
}

export function Modal({ title, onClose, children, footer }: { title: string; onClose: () => void; children: ReactNode; footer?: ReactNode }) {
  return (
    <div className="modal-wrap" onMouseDown={onClose}>
      <div className="modal" onMouseDown={(e) => e.stopPropagation()}>
        <div className="modal-head">
          <h3>{title}</h3>
          <button className="icon-btn" onClick={onClose}>
            <X size={18} />
          </button>
        </div>
        <div className="modal-body">{children}</div>
        {footer && <div className="modal-foot">{footer}</div>}
      </div>
    </div>
  )
}

export function Empty({ children }: { children: ReactNode }) {
  return <div className="empty">{children}</div>
}

export function Field({ label, children, help }: { label: string; children: ReactNode; help?: string }) {
  return (
    <label className="field">
      <span className="field-label">{label}</span>
      {children}
      {help && <span className="field-help">{help}</span>}
    </label>
  )
}

// SideList is the left "maps" panel: saved filters like in an ops tool.
export function SideList({
  title,
  items,
  value,
  onChange,
  footer,
}: {
  title: string
  items: { id: string; title: string; count?: number; icon?: ReactNode }[]
  value: string
  onChange: (id: string) => void
  footer?: ReactNode
}) {
  return (
    <div className="side-list">
      <div className="side-list-title">{title}</div>
      {items.map((it) => (
        <button key={it.id} className={`side-item ${value === it.id ? 'side-item-active' : ''}`} onClick={() => onChange(it.id)}>
          {it.icon}
          <span className="side-item-text">{it.title}</span>
          {it.count !== undefined && <span className="side-count">{it.count}</span>}
        </button>
      ))}
      {footer}
    </div>
  )
}
