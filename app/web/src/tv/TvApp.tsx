import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { applyTheme } from '../theme'
import { tvStrings, type TvLocale } from './strings'
import './tv.css'

type Severity = 'critical' | 'error' | 'warning' | 'info'

type Incident = {
  id: string
  title: string
  ci: string
  severity: Severity
  status: 'open' | 'acknowledged'
  services: string[]
  team?: string
  maintenance: boolean
  opened_at: string
  last_seen: string
  count: number
}

type Board = {
  name: string
  locale: TvLocale
  timezone: string
  theme: 'light' | 'dark'
  refresh: number
  incidents: Incident[]
  by_severity: Partial<Record<Severity, number>>
  more: boolean
  generated_at: string
}

type Failure = { code: string; ip?: string }

const SEVERITIES: Severity[] = ['critical', 'error', 'warning', 'info']
// Retries while the board is refused or the network is down never wait longer than this.
const MAX_RETRY = 30
// How long a page of incidents stays before the list scrolls on, when it does not fit.
const PAGE_MS = 10_000

function browserLocale(): TvLocale {
  return navigator.language?.toLowerCase().startsWith('ru') ? 'ru' : 'en'
}

function slugOf(path: string) {
  return decodeURIComponent(path.replace(/^\/tv\//, '').split('/')[0] ?? '')
}

export default function TvApp() {
  const slug = useMemo(() => slugOf(document.location.pathname), [])
  const [board, setBoard] = useState<Board | null>(null)
  const [failure, setFailure] = useState<Failure | null>(null)
  // offlineSince is when the last good answer came, once requests started failing.
  const [offlineSince, setOfflineSince] = useState<string | null>(null)
  const [unavailable, setUnavailable] = useState(false)
  const [now, setNow] = useState(() => Date.now())
  const timer = useRef<number>(0)
  const last = useRef<Board | null>(null)

  const locale: TvLocale = board?.locale ?? browserLocale()
  const t = useCallback(
    (key: string, vars?: Record<string, string | number>) => {
      let s = tvStrings[locale][key] ?? key
      if (vars) for (const [k, v] of Object.entries(vars)) s = s.split(`{${k}}`).join(String(v))
      return s
    },
    [locale],
  )

  useEffect(() => {
    applyTheme(board?.theme ?? 'dark')
    document.documentElement.lang = locale
    document.title = board ? `${board.name} · Umbrella` : 'Umbrella'
  }, [board?.theme, board?.name, locale])

  useEffect(() => {
    let stopped = false
    const schedule = (seconds: number) => {
      window.clearTimeout(timer.current)
      if (!stopped) timer.current = window.setTimeout(load, seconds * 1000)
    }
    const load = async () => {
      const refresh = last.current?.refresh ?? 15
      try {
        const res = await fetch(`/api/tv/${encodeURIComponent(slug)}`, { cache: 'no-store', credentials: 'omit' })
        const data = await res.json().catch(() => ({}))
        if (res.ok) {
          last.current = data as Board
          setBoard(data as Board)
          setFailure(null)
          setOfflineSince(null)
          setUnavailable(false)
          schedule((data as Board).refresh || 15)
          return
        }
        if (res.status === 404 || res.status === 403) {
          // Refused: the board is gone or this screen is not allowed; nothing old stays on it.
          last.current = null
          setBoard(null)
          setFailure({ code: data.error ?? 'board_not_found', ip: data.ip })
          schedule(MAX_RETRY)
          return
        } else if (res.status === 503) {
          setUnavailable(true)
        } else {
          setOfflineSince((v) => v ?? last.current?.generated_at ?? new Date().toISOString())
        }
      } catch {
        setOfflineSince((v) => v ?? last.current?.generated_at ?? new Date().toISOString())
      }
      schedule(Math.min(refresh, MAX_RETRY))
    }
    void load()
    const tick = window.setInterval(() => setNow(Date.now()), 1000)
    // A screen woken from sleep or reconnected asks at once instead of waiting for the timer.
    const wake = () => document.visibilityState === 'visible' && void load()
    window.addEventListener('online', wake)
    document.addEventListener('visibilitychange', wake)
    return () => {
      stopped = true
      window.clearTimeout(timer.current)
      window.clearInterval(tick)
      window.removeEventListener('online', wake)
      document.removeEventListener('visibilitychange', wake)
    }
  }, [slug])

  const zone = board?.timezone || undefined
  const fmt = useMemo(() => {
    const tag = locale === 'ru' ? 'ru-RU' : 'en-GB'
    const safe = (o: Intl.DateTimeFormatOptions) => {
      try {
        return new Intl.DateTimeFormat(tag, { ...o, timeZone: zone })
      } catch {
        return new Intl.DateTimeFormat(tag, o)
      }
    }
    return {
      clock: safe({ hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }),
      date: safe({ weekday: 'long', day: 'numeric', month: 'long' }),
      time: safe({ hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }),
      short: safe({ day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }),
    }
  }, [locale, zone])

  const age = (v: string) => {
    const s = Math.max(0, Math.round((now - new Date(v).getTime()) / 1000))
    if (s < 60) return t('ago.s')
    if (s < 3600) return t('ago.m', { n: Math.floor(s / 60) })
    if (s < 86400) return t('ago.h', { h: Math.floor(s / 3600), m: Math.floor((s % 3600) / 60) })
    return t('ago.d', { n: Math.floor(s / 86400) })
  }

  if (failure) {
    return (
      <div className="tv tv-center">
        <div className="tv-refused">
          <img src="/logo.svg" alt="" width={72} height={72} />
          <h1>{t('err.title')}</h1>
          <p>{t(`err.${failure.code}`, { ip: failure.ip ?? '' })}</p>
          <p className="tv-muted">{t('err.retry', { n: MAX_RETRY })}</p>
        </div>
      </div>
    )
  }
  if (!board) {
    return (
      <div className="tv tv-center">
        <p className="tv-muted tv-loading">{unavailable ? t('unavailable') : t('loading')}</p>
      </div>
    )
  }

  const total = board.incidents.length
  return (
    <div className="tv">
      <header className="tv-head">
        <div className="tv-title">
          <img src="/logo.svg" alt="" className="tv-logo" />
          <h1>{board.name}</h1>
        </div>
        <div className="tv-clock">
          <span className="tv-clock-time">{fmt.clock.format(now)}</span>
          <span className="tv-clock-date">{fmt.date.format(now)}</span>
        </div>
      </header>
      <section className="tv-counts" aria-label={t('total')}>
        <div className="tv-count tv-count-total">
          <span className="tv-count-n">{total}</span>
          <span className="tv-count-label">{t('total')}</span>
        </div>
        {SEVERITIES.map((s) => (
          <div key={s} className={`tv-count tv-sev-${s} ${board.by_severity[s] ? 'on' : 'zero'}`}>
            <span className="tv-count-n">{board.by_severity[s] ?? 0}</span>
            <span className="tv-count-label">{t(`count.${s}`)}</span>
          </div>
        ))}
      </section>
      {offlineSince && <div className="tv-banner tv-banner-offline">{t('offline', { time: fmt.time.format(new Date(offlineSince)) })}</div>}
      {unavailable && <div className="tv-banner">{t('unavailable')}</div>}
      {total === 0 ? (
        <div className="tv-clear">
          <span className="tv-clear-mark" aria-hidden />
          <h2>{t('clear')}</h2>
          <p className="tv-muted">{t('clear.sub')}</p>
        </div>
      ) : (
        <Pager key={board.incidents.map((i) => i.id).join(',')}>
          {board.incidents.map((i) => (
            <article key={i.id} className={`tv-row tv-sev-${i.severity} ${i.status === 'acknowledged' ? 'tv-acked' : ''}`}>
              <div className="tv-row-sev">{t(`sev.${i.severity}`)}</div>
              <div className="tv-row-main">
                <div className="tv-row-title">{i.title}</div>
                <div className="tv-row-meta">
                  <span className="tv-row-ci">{i.ci}</span>
                  {i.services.length > 0 && <span>{i.services.join(', ')}</span>}
                  {i.team && <span>{i.team}</span>}
                  {i.status === 'acknowledged' && <span className="tv-tag tv-tag-ack">{t('acked')}</span>}
                  {i.maintenance && <span className="tv-tag tv-tag-mw">{t('maintenance')}</span>}
                </div>
              </div>
              <div className="tv-row-time">
                <span className="tv-row-age">{age(i.opened_at)}</span>
                <span className="tv-muted">{t('since', { time: fmt.short.format(new Date(i.opened_at)) })}</span>
              </div>
            </article>
          ))}
          {board.more && <p className="tv-muted tv-more">{t('more', { n: total })}</p>}
        </Pager>
      )}
      <footer className="tv-foot">
        <span className={`tv-live ${offlineSince ? 'off' : ''}`}>
          <span className="tv-live-dot" aria-hidden />
          {offlineSince ? t('offline.short') : t('live')}
        </span>
        <span>{t('updated', { time: fmt.time.format(new Date(board.generated_at)) })}</span>
      </footer>
    </div>
  )
}

// Pager shows a list that does not fit one screen page by page, returning to the top at the end.
function Pager({ children }: { children: ReactNode }) {
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const el = ref.current
    if (!el) return
    const id = window.setInterval(() => {
      if (el.scrollHeight <= el.clientHeight + 4) return
      const next = el.scrollTop + el.clientHeight >= el.scrollHeight - 4 ? 0 : el.scrollTop + el.clientHeight - 40
      el.scrollTo({ top: next, behavior: 'smooth' })
    }, PAGE_MS)
    return () => window.clearInterval(id)
  }, [])
  return (
    <div ref={ref} className="tv-list">
      {children}
    </div>
  )
}
