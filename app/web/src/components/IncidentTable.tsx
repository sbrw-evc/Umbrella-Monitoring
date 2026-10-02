import { fmtDuration, fmtTime, methodLabel, type Incident } from '../api'
import { t } from '../i18n'
import { Empty, PDPill, SevBadge, StatusPill } from './ui'

export type SortKey = 'severity' | 'id' | 'ci' | 'service' | 'first_seen' | 'last_seen' | 'count'

export function IncidentTable({
  items,
  selected,
  onSelect,
  onOpen,
  sort,
  desc,
  onSort,
  compact,
}: {
  items: Incident[]
  selected?: Set<string>
  onSelect?: (s: Set<string>) => void
  onOpen: (id: string) => void
  sort?: SortKey
  desc?: boolean
  onSort?: (k: SortKey) => void
  compact?: boolean
}) {
  if (items.length === 0) return <Empty>{t('incidents.table.empty')}</Empty>
  const allChecked = !!selected && items.length > 0 && items.every((i) => selected.has(i.id))
  const th = (k: SortKey, title: string) => (
    <th className={onSort ? 'sortable' : ''} onClick={() => onSort?.(k)}>
      {title}
      {sort === k && <span className="sort-arrow">{desc ? '▼' : '▲'}</span>}
    </th>
  )
  return (
    <table className={`table ${compact ? 'table-compact' : ''}`}>
      <thead>
        <tr>
          {selected && (
            <th className="col-check">
              <input
                type="checkbox"
                checked={allChecked}
                onChange={() => onSelect?.(allChecked ? new Set() : new Set(items.map((i) => i.id)))}
              />
            </th>
          )}
          {th('severity', t('incidents.table.severity'))}
          {th('id', 'ID')}
          <th>{t('incidents.table.incident')}</th>
          {th('ci', t('incidents.table.ci'))}
          <th>{t('common.words.status')}</th>
          <th>PagerDuty</th>
          {th('count', t('incidents.table.events'))}
          {th('first_seen', t('incidents.table.opened'))}
        </tr>
      </thead>
      <tbody>
        {items.map((a) => (
          <tr key={a.id} className={`row-sev row-${a.severity} ${a.status === 'resolved' ? 'row-resolved' : ''}`} onClick={() => onOpen(a.id)}>
            {selected && (
              <td className="col-check" onClick={(e) => e.stopPropagation()}>
                <input
                  type="checkbox"
                  checked={selected.has(a.id)}
                  onChange={() => {
                    const s = new Set(selected)
                    if (s.has(a.id)) s.delete(a.id)
                    else s.add(a.id)
                    onSelect?.(s)
                  }}
                />
              </td>
            )}
            <td>
              <SevBadge sev={a.severity} />
            </td>
            <td className="mono nowrap">{a.id}</td>
            <td className="cell-title">
              <a
                className="title-line title-link"
                href={`/go/incidents/${a.id}/grafana`}
                target="_blank"
                rel="noreferrer"
                title={t('incidents.table.grafanaTitle')}
                onClick={(e) => e.stopPropagation()}
              >
                {a.title}
              </a>
              <div className="sub-line">
                <code>{a.signal}</code>
                {a.method !== 'other' && <span className={`tag tag-${a.method}`}>{methodLabel(a.method)}</span>}
                {a.related_id && <span className="tag tag-link">{t('incidents.table.relatedTo', { id: a.related_id })}</span>}
                {a.suppressed && <span className="tag">{t('incidents.table.suppressed')}</span>}
              </div>
            </td>
            <td className="cell-ci">
              <div className="title-line">
                {a.ci_name || '—'}
                {!a.ci_id && <span className="tag tag-warn">{t('incidents.table.noCi')}</span>}
              </div>
              {!compact && a.service && <div className="sub-line" title={t('incidents.table.service')}>{a.service}</div>}
            </td>
            <td>
              <StatusPill status={a.status} />
            </td>
            <td>
              <PDPill state={a.pd_state} fallback={a.fallback} />
            </td>
            <td className="num">{a.count}</td>
            <td className="nowrap">
              {fmtTime(a.first_seen)}
              {!compact && <div className="sub-line" title={t('incidents.table.duration')}>{fmtDuration(a.first_seen, a.resolved_at)}</div>}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
