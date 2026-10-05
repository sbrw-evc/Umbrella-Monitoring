import { useCallback, useEffect, useState } from 'react'
import { Plus, Search } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { ErrorBanner } from '../../connections/ConnectionCard'
import { useResource } from '../../connections/useRequest'
import { useT } from '../../i18n'
import { Button, Input, Select } from '../../ui'
import { useSession } from '../session'
import { SourcePill, StatusPill } from './Badges'
import { CIDetail } from './CIDetail'
import { CIEditor, DeleteDialog } from './CIEditor'
import { strings } from './strings'
import { FLAGS, KINDS, NO_FILTERS, queryOf, SOURCES, STATUSES, type CI, type CIList, type Filters, type Summary } from './types'
import '../services/services.css'
import '../connectors/connectors.css'
import './cis.css'

function useDebounced<T>(value: T, ms: number) {
  const [v, setV] = useState(value)
  useEffect(() => {
    const id = window.setTimeout(() => setV(value), ms)
    return () => window.clearTimeout(id)
  }, [value, ms])
  return v
}

export function CIsPage() {
  const t = useT(strings)
  const { can } = useSession()
  const editable = can('cis:edit')
  const [epoch, setEpoch] = useState(0)
  const [filters, setFilters] = useState<Filters>(NO_FILTERS)
  const [selected, setSelected] = useState<CI | null>(null)
  const [editing, setEditing] = useState<{ ci: CI | null } | null>(null)
  const [deleting, setDeleting] = useState<CI | null>(null)
  const q = useDebounced(filters.q, 250)
  const list = useResource<CIList>(`/api/cis${queryOf({ ...filters, q })}`, epoch)
  const reload = useCallback(() => setEpoch((e) => e + 1), [])
  const set = (patch: Partial<Filters>) => setFilters((f) => ({ ...f, ...patch }))
  const filtered = Object.values(filters).some((v) => v.trim() !== '')

  if (!list.data) {
    return list.error ? <ErrorBanner error={list.error} strings={strings} /> : <p className="muted">{t('loading')}</p>
  }
  const { items, summary } = list.data

  return (
    <div className="ci-page">
      <Tiles summary={summary} flag={filters.flag} onFlag={(flag) => set({ flag: filters.flag === flag ? '' : flag })} />
      <div className="card svc-toolbar">
        <div className="svc-toolbar-row">
          <label className="svc-search">
            <Search size={16} aria-hidden />
            <Input type="search" value={filters.q} placeholder={t('ci.search')} aria-label={t('ci.search')} onChange={(e) => set({ q: e.target.value })} />
          </label>
          {editable && (
            <Button variant="primary" onClick={() => setEditing({ ci: null })}>
              <Plus size={16} />
              {t('ci.create')}
            </Button>
          )}
        </div>
        <div className="svc-filters">
          <FilterSelect label={t('ci.filter.kind')} value={filters.kind} values={KINDS} prefix="ci.kind" onChange={(kind) => set({ kind })} />
          <FilterSelect label={t('ci.filter.status')} value={filters.status} values={STATUSES} prefix="ci.status" onChange={(status) => set({ status })} />
          <FilterSelect label={t('ci.filter.source')} value={filters.source} values={SOURCES} prefix="ci.source" onChange={(source) => set({ source })} />
        </div>
        <div className="svc-toolbar-row">
          <span />
          <div className="row">
            {filtered && (
              <Button variant="ghost" onClick={() => setFilters(NO_FILTERS)}>
                {t('ci.filter.reset')}
              </Button>
            )}
            <span className="muted svc-count">{t('ci.count', { shown: items.length, total: summary.total })}</span>
          </div>
        </div>
      </div>

      {!editable && summary.total > 0 && <p className="muted">{t('ci.readonly')}</p>}

      <AnimatePresence mode="wait" initial={false}>
        {items.length === 0 ? (
          <motion.div key="empty" className="card svc-empty" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}>
            <p>{t(filtered ? 'ci.empty.filtered' : 'ci.empty')}</p>
            {!filtered && editable && <p className="muted">{t('ci.empty.edit')}</p>}
          </motion.div>
        ) : (
          <motion.div key="list" className="card cn-table-wrap" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}>
            <CITable items={items} onOpen={setSelected} />
          </motion.div>
        )}
      </AnimatePresence>

      <CIDetail
        ci={selected}
        editable={editable}
        onClose={() => setSelected(null)}
        onEdit={(ci) => {
          setSelected(null)
          setEditing({ ci })
        }}
        onDelete={(ci) => {
          setSelected(null)
          setDeleting(ci)
        }}
        onChanged={(ci) => {
          setSelected(ci)
          reload()
        }}
      />
      <CIEditor
        open={editing !== null}
        ci={editing?.ci ?? null}
        tags={list.data.tags}
        onClose={() => setEditing(null)}
        onSaved={(ci) => {
          setEditing(null)
          setSelected(ci)
          reload()
        }}
      />
      <DeleteDialog
        ci={deleting}
        onClose={() => setDeleting(null)}
        onDeleted={() => {
          setDeleting(null)
          reload()
        }}
      />
    </div>
  )
}

function FilterSelect({
  label,
  value,
  values,
  prefix,
  onChange,
}: {
  label: string
  value: string
  values: readonly string[]
  prefix: string
  onChange: (v: string) => void
}) {
  const t = useT(strings)
  return (
    <label>
      <span>{label}</span>
      <Select value={value} onChange={(e) => onChange(e.target.value)}>
        <option value="">{t('ci.filter.any')}</option>
        {values.map((v) => (
          <option key={v} value={v}>
            {t(`${prefix}.${v}`)}
          </option>
        ))}
      </Select>
    </label>
  )
}

function Tiles({ summary, flag, onFlag }: { summary: Summary; flag: string; onFlag: (flag: string) => void }) {
  const t = useT(strings)
  return (
    <div className="ci-tiles">
      <div className="card ci-tile">
        <span className="ci-tile-value">{summary.total}</span>
        <span className="muted">{t('ci.tile.total')}</span>
      </div>
      <div className="card ci-tile">
        <span className="ci-tile-value">{summary.netbox}</span>
        <span className="muted">{t('ci.tile.netbox')}</span>
      </div>
      <div className="card ci-tile">
        <span className="ci-tile-value">{summary.local}</span>
        <span className="muted">
          {t('ci.tile.local')}
          {summary.registered > 0 && <> · {t('ci.tile.registered', { n: summary.registered })}</>}
        </span>
      </div>
      {FLAGS.map((f) => (
        <button
          key={f}
          type="button"
          className={`card ci-tile ci-tile-flag ${flag === f ? 'active' : ''} ${summary[f] > 0 ? 'warn' : ''}`}
          aria-pressed={flag === f}
          onClick={() => onFlag(f)}
        >
          <span className="ci-tile-value">{summary[f]}</span>
          <span className="muted">{t(`ci.tile.${f}`)}</span>
        </button>
      ))}
    </div>
  )
}

function CITable({ items, onOpen }: { items: CI[]; onOpen: (ci: CI) => void }) {
  const t = useT(strings)
  return (
    <table className="cn-table ci-table">
      <thead>
        <tr>
          <th>{t('ci.col.name')}</th>
          <th>{t('ci.col.kind')}</th>
          <th>{t('ci.col.status')}</th>
          <th>{t('ci.col.ips')}</th>
          <th>{t('ci.col.owners')}</th>
          <th>{t('ci.col.source')}</th>
        </tr>
      </thead>
      <tbody>
        {items.map((ci) => (
          <tr key={ci.id}>
            <td>
              <button type="button" className="cn-link cn-name" onClick={() => onOpen(ci)}>
                {ci.name}
              </button>
              {(ci.attrs.site || ci.attrs.parent) && <div className="muted">{ci.attrs.parent || ci.attrs.site}</div>}
            </td>
            <td>{t(`ci.kind.${ci.kind}`)}</td>
            <td>
              <StatusPill status={ci.status} />
            </td>
            <td className="cn-mono">{ci.ips.join(', ') || t('ci.none')}</td>
            <td>{ci.owners.length ? ci.owners.map((o) => o.name || o.username || t('ci.owner.deleted')).join(', ') : <span className="muted">{t('ci.none')}</span>}</td>
            <td>
              <SourcePill ci={ci} />
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
