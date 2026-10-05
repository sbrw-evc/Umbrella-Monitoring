import type { ReactNode } from 'react'
import { PlugZap } from 'lucide-react'
import { errorText, useT, type Dict } from '../i18n'
import { Banner, Button, Rows } from '../ui'
import { connectionStrings } from './connectionStrings'
import './connections.css'

export function StatePill({ ok }: { ok: boolean }) {
  const t = useT(connectionStrings)
  return <span className={`pill pill-${ok ? 'ok' : 'error'}`}>{t(ok ? 'conn.state.ok' : 'conn.state.error')}</span>
}

export function ErrorBanner({ error, strings }: { error: unknown; strings?: Dict }) {
  const t = useT(strings ?? connectionStrings)
  if (!error) return null
  const e = errorText(t, error)
  return (
    <Banner kind="error" title={e.message}>
      {e.detail}
    </Banner>
  )
}

export function ConnectionCard({
  title,
  ok,
  rows,
  problem,
  onTest,
  testing,
  children,
}: {
  title: string
  ok: boolean
  rows: [ReactNode, ReactNode][]
  problem?: string
  onTest?: () => void
  testing: boolean
  children?: ReactNode
}) {
  const t = useT(connectionStrings)
  return (
    <section className="card status-card" aria-label={title}>
      <header>
        <h2>{title}</h2>
        <StatePill ok={ok} />
      </header>
      <Rows rows={rows} />
      {problem && <Banner kind="error" title={problem} />}
      {children}
      {onTest && (
        <div className="card-actions">
          <Button onClick={onTest} busy={testing}>
            {!testing && <PlugZap size={16} />}
            {t('conn.test')}
          </Button>
        </div>
      )}
    </section>
  )
}

export function MigrationCard({ title, text, children }: { title: string; text: string; children: ReactNode }) {
  return (
    <section className="card status-card" aria-label={title}>
      <header className="conn-head">
        <div>
          <h2>{title}</h2>
          <p className="muted">{text}</p>
        </div>
      </header>
      {children}
    </section>
  )
}

export function MigrationActions({
  onCheck,
  canCheck,
  onMigrate,
  canMigrate,
  busy,
}: {
  onCheck: () => void
  canCheck: boolean
  onMigrate: () => void
  canMigrate: boolean
  busy: boolean
}) {
  const t = useT(connectionStrings)
  return (
    <div className="conn-actions">
      <Button onClick={onCheck} busy={busy} disabled={!canCheck}>
        {t('conn.check')}
      </Button>
      <Button variant="primary" onClick={onMigrate} disabled={!canMigrate || busy}>
        {t('conn.migrate')}
      </Button>
    </div>
  )
}
