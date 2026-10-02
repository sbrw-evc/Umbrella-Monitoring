import { Check, Download, Languages, Palette, Sparkles, Trash2, Upload } from 'lucide-react'
import { useRef, useState } from 'react'
import { useApp } from '../context'
import { loadMotion, saveMotion, type Motion } from '../motion'
import { BUILT_IN_LOCALES, coverage, download, localeTemplate, parseLocale, t, type Dict } from '../i18n'
import { PageHeader, Tabs } from '../components/ui'
import { parseTheme, resolveColors, themeTemplate, THEME_GROUP_HELP, BUILT_IN_THEMES, type Theme } from '../theme'

type Tab = 'themes' | 'languages'

export function SettingsPage() {
  const [tab, setTab] = useState<Tab>(() => (location.hash === '#languages' ? 'languages' : 'themes'))
  return (
    <div className="page">
      <div className="page-main page-narrow">
        <PageHeader title={t('settings.header.title')} sub={t('settings.header.sub')} />
        <Tabs
          tabs={[
            { id: 'themes', title: t('settings.tabs.themes'), icon: <Palette size={16} /> },
            { id: 'languages', title: t('settings.tabs.languages'), icon: <Languages size={16} /> },
          ]}
          value={tab}
          onChange={(v) => {
            setTab(v)
            history.replaceState(null, '', `#${v}`)
          }}
        />
        {tab === 'themes' ? <ThemesSection /> : <LanguagesSection />}
        <MotionSection />
      </div>
    </div>
  )
}

function MotionSection() {
  const [motion, setMotion] = useState<Motion>(loadMotion)
  return (
    <section className="card">
      <div className="card-head">
        <div>
          <h3>
            <Sparkles size={16} /> {t('settings.motion.title')}
          </h3>
          <p className="hint">{t('settings.motion.intro')}</p>
        </div>
        <div className="seg">
          {(['auto', 'on', 'off'] as Motion[]).map((m) => (
            <button
              key={m}
              className={`seg-btn ${motion === m ? 'seg-active' : ''}`}
              onClick={() => {
                setMotion(m)
                saveMotion(m)
              }}
            >
              {t(`settings.motion.${m}`)}
            </button>
          ))}
        </div>
      </div>
    </section>
  )
}

// useFilePicker opens a file dialog and hands over the chosen file's text.
function useFilePicker(onText: (text: string) => void) {
  const ref = useRef<HTMLInputElement>(null)
  const input = (
    <input
      ref={ref}
      type="file"
      accept="application/json,.json"
      hidden
      onChange={(e) => {
        const f = e.target.files?.[0]
        e.target.value = ''
        if (f) f.text().then(onText)
      }}
    />
  )
  return { input, open: () => ref.current?.click() }
}

function themeTitle(th: Theme) {
  return th.builtIn ? t(`settings.themes.builtIn.${th.id}`) : th.name
}

function ThemesSection() {
  const { themes, theme, setTheme, addTheme, removeTheme, toast } = useApp()
  const [base, setBase] = useState<'light' | 'dark'>(theme.base)
  const [error, setError] = useState('')
  const picker = useFilePicker((text) => {
    try {
      const th = parseTheme(text)
      addTheme(th)
      setTheme(th.id)
      setError('')
      toast(t('settings.themes.added', { name: th.name }))
    } catch (e) {
      setError(t('settings.themes.rejected', { error: (e as Error).message }))
    }
  })
  const ref = resolveColors(BUILT_IN_THEMES[0])

  return (
    <>
      <section className="card">
        <div className="card-head">
          <div>
            <h3>{t('settings.themes.title')}</h3>
            <p className="hint">{t('settings.themes.intro')}</p>
          </div>
        </div>
        <div className="theme-grid">
          {themes.map((th) => {
            const c = resolveColors(th)
            const active = th.id === theme.id
            return (
              <div key={th.id} className={`theme-card ${active ? 'theme-card-active' : ''}`}>
                <button className="theme-preview" onClick={() => setTheme(th.id)} style={{ background: c.app.background, borderColor: c.app.border }}>
                  <span className="tp-rail" style={{ background: c.rail.background, borderColor: c.rail.border }}>
                    <i style={{ background: c.rail.iconActive }} />
                    <i style={{ background: c.rail.icon }} />
                    <i style={{ background: c.rail.icon }} />
                  </span>
                  <span className="tp-body">
                    <span className="tp-bar" style={{ background: c.topbar.background, borderColor: c.topbar.border }} />
                    <span className="tp-card" style={{ background: c.app.surface, borderColor: c.app.border }}>
                      <span className="tp-line" style={{ background: c.app.text }} />
                      <span className="tp-line tp-line-short" style={{ background: c.app.textMuted }} />
                      <span className="tp-chips">
                        {(['critical', 'error', 'warning', 'info', 'ok'] as const).map((s) => (
                          <i key={s} style={{ background: c.severity[s] }} />
                        ))}
                      </span>
                      <span className="tp-btn" style={{ background: c.accent.primary }} />
                    </span>
                  </span>
                </button>
                <div className="theme-meta">
                  <div className="theme-name">
                    {themeTitle(th)}
                    <span className="tag">{th.builtIn ? t('settings.themes.badgeBuiltIn') : t('settings.themes.badgeCustom')}</span>
                  </div>
                  <div className="theme-actions">
                    {active ? (
                      <span className="theme-active">
                        <Check size={14} /> {t('settings.themes.active')}
                      </span>
                    ) : (
                      <button className="btn btn-sm" onClick={() => setTheme(th.id)}>
                        {t('settings.themes.use')}
                      </button>
                    )}
                    <button className="icon-btn" title={t('settings.themes.export')} onClick={() => download(`umbrella-theme-${th.id}.json`, themeTemplateFor(th))}>
                      <Download size={16} />
                    </button>
                    {!th.builtIn && (
                      <button
                        className="icon-btn"
                        title={t('settings.themes.remove')}
                        onClick={() => window.confirm(t('settings.themes.confirmRemove', { name: th.name })) && removeTheme(th.id)}
                      >
                        <Trash2 size={16} />
                      </button>
                    )}
                  </div>
                </div>
              </div>
            )
          })}
        </div>
        <div className="toolbar">
          <label className="inline-field">
            <span>{t('settings.themes.baseOn')}</span>
            <select value={base} onChange={(e) => setBase(e.target.value as 'light' | 'dark')}>
              <option value="light">{t('settings.themes.builtIn.light')}</option>
              <option value="dark">{t('settings.themes.builtIn.dark')}</option>
            </select>
          </label>
          <button className="btn" onClick={() => download(`umbrella-theme-template-${base}.json`, themeTemplate(BUILT_IN_THEMES.find((x) => x.id === base)!))}>
            <Download size={15} /> {t('settings.themes.downloadTemplate')}
          </button>
          <button className="btn btn-primary" onClick={picker.open}>
            <Upload size={15} /> {t('settings.themes.upload')}
          </button>
          {picker.input}
        </div>
        {error && <div className="form-error">{error}</div>}
      </section>

      <section className="card">
        <h3>{t('settings.themes.structure')}</h3>
        <p className="hint">{t('settings.themes.structureHint')}</p>
        <div className="schema-list">
          {Object.entries(ref).map(([g, tokens]) => (
            <div key={g} className="schema-row">
              <code>{g}</code>
              <span className="schema-desc">{tr(`settings.themes.groups.${g}`, THEME_GROUP_HELP[g])}</span>
              <span className="schema-count">{t('settings.themes.tokens', { n: Object.keys(tokens).length })}</span>
              <span className="schema-swatches">
                {Object.entries(resolveColors(theme)[g])
                  .slice(0, 8)
                  .map(([k, v]) => (
                    <i key={k} title={`${g}.${k}: ${v}`} style={{ background: v }} />
                  ))}
              </span>
            </div>
          ))}
        </div>
      </section>
    </>
  )
}

// A theme's own JSON: the template layout with this theme's colors.
function themeTemplateFor(th: Theme) {
  const tpl = themeTemplate(th) as Record<string, unknown>
  return { ...tpl, id: th.builtIn ? `${th.id}-custom` : th.id, name: th.builtIn ? `${themeTitle(th)} (copy)` : th.name }
}

// tr translates a key and falls back to a default when it is missing.
function tr(key: string, def: string) {
  const v = t(key)
  return v === key ? def : v
}

function LanguagesSection() {
  const { locales, locale, setLocale, addLocale, removeLocale, toast } = useApp()
  const [base, setBase] = useState(locale.id === 'ru' ? 'ru' : 'en')
  const [error, setError] = useState('')
  const ref = BUILT_IN_LOCALES.find((l) => l.id === 'ru')!.messages
  const picker = useFilePicker((text) => {
    try {
      const l = parseLocale(text)
      // The success toast is shown after the remount, in the new language.
      addLocale(l)
      setLocale(l.id)
      window.setTimeout(() => toast(t('settings.languages.added', { name: l.name })), 50)
    } catch (e) {
      setError(t('settings.languages.rejected', { error: (e as Error).message }))
    }
  })

  return (
    <>
      <section className="card">
        <div className="card-head">
          <div>
            <h3>{t('settings.languages.title')}</h3>
            <p className="hint">{t('settings.languages.intro')}</p>
          </div>
        </div>
        <table className="table table-plain">
          <tbody>
            {locales.map((l) => {
              const cov = coverage(l.messages, ref)
              const pct = cov.total ? Math.round((cov.covered / cov.total) * 100) : 0
              const custom = !l.builtIn
              return (
                <tr key={l.id} className={l.id === locale.id ? 'row-selected' : ''}>
                  <td className="lang-name">
                    <b>{l.name}</b>
                    <span className="tag">{custom ? t('settings.languages.badgeCustom') : t('settings.languages.badgeBuiltIn')}</span>
                  </td>
                  <td className="mono muted nowrap">
                    {t('settings.languages.code')}: {l.id} · {l.dateLocale}
                  </td>
                  <td>
                    <div className="lang-cov">
                      <span className="meter">
                        <span style={{ width: `${pct}%` }} />
                      </span>
                      <span className="muted nowrap">{t('settings.languages.coverage', { covered: cov.covered, total: cov.total })}</span>
                    </div>
                  </td>
                  <td className="nowrap right">
                    {l.id === locale.id ? (
                      <span className="theme-active">
                        <Check size={14} /> {t('settings.languages.active')}
                      </span>
                    ) : (
                      <button className="btn btn-sm" onClick={() => setLocale(l.id)}>
                        {t('settings.languages.use')}
                      </button>
                    )}
                    <button className="icon-btn" title={t('settings.languages.export')} onClick={() => download(`umbrella-locale-${l.id}.json`, { ...localeTemplate(l), id: l.id, name: l.name, dateLocale: l.dateLocale })}>
                      <Download size={16} />
                    </button>
                    {custom && (
                      <button
                        className="icon-btn"
                        title={t('settings.languages.remove')}
                        onClick={() => window.confirm(t('settings.languages.confirmRemove', { name: l.name })) && removeLocale(l.id)}
                      >
                        <Trash2 size={16} />
                      </button>
                    )}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
        <div className="toolbar">
          <label className="inline-field">
            <span>{t('settings.languages.baseOn')}</span>
            <select value={base} onChange={(e) => setBase(e.target.value)}>
              {BUILT_IN_LOCALES.map((l) => (
                <option key={l.id} value={l.id}>
                  {l.name}
                </option>
              ))}
            </select>
          </label>
          <button className="btn" onClick={() => download(`umbrella-locale-template-${base}.json`, localeTemplate(BUILT_IN_LOCALES.find((l) => l.id === base)!))}>
            <Download size={15} /> {t('settings.languages.downloadTemplate')}
          </button>
          <button className="btn btn-primary" onClick={picker.open}>
            <Upload size={15} /> {t('settings.languages.upload')}
          </button>
          {picker.input}
        </div>
        {error && <div className="form-error">{error}</div>}
      </section>

      <section className="card">
        <h3>{t('settings.languages.structure')}</h3>
        <p className="hint">{t('settings.languages.structureHint')}</p>
        <div className="schema-list">
          {Object.entries(ref).map(([section, d]) => (
            <div key={section} className="schema-row">
              <code>{section}</code>
              <span className="schema-desc">{Object.keys(d as Dict).join(', ')}</span>
              <span className="schema-count">{t('settings.languages.strings', { n: coverage(d as Dict, d as Dict).total })}</span>
            </div>
          ))}
        </div>
      </section>
    </>
  )
}
