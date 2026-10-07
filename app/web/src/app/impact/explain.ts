import { severityText, type Impact } from '../incidents/types'

type T = (key: string, vars?: Record<string, string | number>) => string

// The impact levels of the matrix (model.ImpactLevels), most critical first.
export const LEVELS = ['critical', 'high', 'medium', 'low', 'none'] as const
export type Level = (typeof LEVELS)[number]

export type RuleAction = 'set' | 'raise' | 'lower' | 'impact'

// Rule is model.ClassificationRule.
export type Rule = {
  id: string
  name: string
  enabled: boolean
  when: string
  action: RuleAction
  severity?: string
  n?: number
  level?: string
  stop?: boolean
}

// Policy is model.ImpactPolicy: matrix[event severity][impact level] = priority.
export type Policy = {
  configured: boolean
  enabled: boolean
  upstream_depth: number
  matrix: Record<string, Record<string, string>>
  rules: Rule[]
}

// levelText names an impact level; empty is unknown.
export function levelText(t: T, level: string | undefined) {
  const key = `im.level.${level || 'unknown'}`
  const s = t(key)
  return s === key ? (level ?? '') : s
}

// stepText renders a step of Impact.steps; unknown codes show as they are.
export function stepText(t: T, step: NonNullable<Impact['steps']>[number]) {
  const a: Record<string, string> = { ...(step.args ?? {}) }
  for (const k of ['severity', 'event', 'from']) if (a[k]) a[k] = severityText(t, a[k])
  if (a.level) a.level = levelText(t, a.level)
  let key = `im.step.${step.code}`
  if (step.code === 'impact_level') {
    if (step.args?.services && step.args?.upstream) key += '.upstream'
    else if (step.args?.services) key += '.services'
    else if (step.args?.upstream) key += '.only_upstream'
  }
  if (step.code === 'impact_unknown') key += `.${step.args?.reason ?? 'ci_unknown'}`
  const out = t(key, a)
  return out === key ? [step.code, ...Object.entries(step.args ?? {}).map(([k, v]) => `${k}=${v}`)].join(' ') : out
}

// ruleSummary is the action of a rule in a few words.
export function ruleSummary(t: T, r: Rule) {
  switch (r.action) {
    case 'set':
      return t('im.action.set.text', { sev: severityText(t, r.severity ?? '') })
    case 'raise':
    case 'lower':
      return t(`im.action.${r.action}.text`, { n: r.n ?? 1 })
    case 'impact':
      return t('im.action.impact.text', { level: levelText(t, r.level) })
  }
  return r.action
}
