import { Info } from 'lucide-react'
import { fmtDuration, fmtTime, METHOD_LABEL, type Incident } from '../api'
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
  if (items.length === 0) return <Empty>Инцидентов по этим условиям нет</Empty>
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
          {th('severity', 'Severity')}
          {th('id', 'ID')}
          <th>Инцидент</th>
          {th('ci', 'КЕ')}
          {!compact && th('service', 'ИТ-сервис')}
          <th>Метод</th>
          <th>Статус</th>
          <th>PagerDuty</th>
          {th('count', 'Событий')}
          {th('first_seen', 'Открыт')}
          {!compact && <th>Длительность</th>}
          <th />
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
                title="Открыть контекст инцидента в Grafana"
                onClick={(e) => e.stopPropagation()}
              >
                {a.title}
              </a>
              <div className="sub-line">
                <code>{a.signal}</code>
                {a.related_id && <span className="tag tag-link">связан с {a.related_id}</span>}
                {a.suppressed && <span className="tag">подавлен</span>}
              </div>
            </td>
            <td className="nowrap">{a.ci_name || '—'}{!a.ci_id && <span className="tag tag-warn">без КЕ</span>}</td>
            {!compact && <td>{a.service || '—'}</td>}
            <td>{METHOD_LABEL[a.method]}</td>
            <td>
              <StatusPill status={a.status} />
            </td>
            <td>
              <PDPill state={a.pd_state} fallback={a.fallback} />
            </td>
            <td className="num">{a.count}</td>
            <td className="nowrap">{fmtTime(a.first_seen)}</td>
            {!compact && <td className="nowrap">{fmtDuration(a.first_seen, a.resolved_at)}</td>}
            <td>
              <button
                className="icon-btn"
                title="Подробнее"
                onClick={(e) => {
                  e.stopPropagation()
                  onOpen(a.id)
                }}
              >
                <Info size={16} />
              </button>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
