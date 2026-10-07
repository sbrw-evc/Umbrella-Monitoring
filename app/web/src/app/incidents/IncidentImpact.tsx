import { Scale } from 'lucide-react'
import { useT } from '../../i18n'
import { levelText, stepText } from '../impact/explain'
import { strings } from '../impact/strings'
import '../impact/impact.css'
import { severityTone } from './format'
import { severityText, type Impact, type Incident } from './types'

// ImpactLine is the priority and how it was found, in one line:
// "Priority P2 · High ← event P3 · Medium × impact Critical (Payments, upstream: Checkout) · rule «DB»".
export function ImpactLine({ severity, event, impact }: { severity: string; event: string; impact: Impact }) {
  const t = useT(strings)
  const direct = (impact.services ?? []).filter((s) => s.direct).map((s) => s.name)
  const upstream = (impact.services ?? []).filter((s) => !s.direct).map((s) => s.name)
  const names = [direct.join(', '), upstream.length ? `${t('im.upstream')}: ${upstream.join(', ')}` : ''].filter(Boolean).join('; ')
  return (
    <p className="im-line">
      <span className={`pill inc-sev inc-sev-${severityTone(severity)}`}>
        <span className="inc-dot" aria-hidden />
        {t('im.priority')} {severityText(t, severity)}
      </span>
      <span className="im-from">
        ← {t('im.event')} <b>{severityText(t, event)}</b> × {t('im.impact')} <b>{levelText(t, impact.level)}</b>
        {names && <> ({names})</>}
        {impact.rule && (
          <>
            {' '}
            · {t('im.rule')} «{impact.rule}»
          </>
        )}
      </span>
    </p>
  )
}

// ImpactSteps lists how the priority was found, step by step.
export function ImpactSteps({ impact }: { impact: Impact }) {
  const t = useT(strings)
  return (
    <ol className="im-steps">
      {(impact.steps ?? []).map((s, i) => (
        <li key={i} className={s.code === 'rule_error' ? 'im-step-error' : undefined}>
          {stepText(t, s)}
        </li>
      ))}
    </ol>
  )
}

// IncidentImpact explains the priority of the incident: event severity, business impact and
// the rule that changed it.
export function IncidentImpact({ a }: { a: Incident }) {
  const t = useT(strings)
  if (!a.impact) return null
  return (
    <section className="im-inc" aria-label={t('im.priority')}>
      <Scale size={16} aria-hidden className="im-inc-icon" />
      <div className="im-inc-body">
        <ImpactLine severity={a.severity} event={a.event_severity ?? a.severity} impact={a.impact} />
        {(a.impact.steps?.length ?? 0) > 0 && (
          <details className="im-details">
            <summary>{t('im.how')}</summary>
            <ImpactSteps impact={a.impact} />
          </details>
        )}
      </div>
    </section>
  )
}
