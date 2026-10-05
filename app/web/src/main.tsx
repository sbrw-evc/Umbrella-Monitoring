import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { type ReactNode } from 'react'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { Layout, ROUTE_PERMS } from './components/Layout'
import { AppProvider, useApp } from './context'
import { NotificationsPage, RolesPage, UsersPage } from './pages/Access'
import { CmdbPage } from './pages/Cmdb'
import { ConnectorEditorPage } from './pages/ConnectorEditor'
import { ConnectorsPage } from './pages/Connectors'
import { HeatmapPage } from './pages/Heatmap'
import { IncidentsPage } from './pages/Incidents'
import { AuditPage, EventsPage, MaintenancePage, ParseErrorsPage, SelfCheckPage } from './pages/Misc'
import { CisPage } from './pages/Cis'
import { IntegrationsPage } from './pages/Integrations'
import { RulesPage } from './pages/Rules'
import { TeamsPage } from './pages/Teams'
import { OpsPage } from './pages/Ops'
import { SettingsPage } from './pages/Settings'
import { applyTheme, BUILT_IN_THEMES, loadActiveTheme, loadCustomThemes } from './theme'
import { applyMotion, loadMotion } from './motion'
import './styles.css'

// Paint the saved theme before the first render so the page does not flash.
{
  applyMotion(loadMotion())
  const id = loadActiveTheme()
  applyTheme([...BUILT_IN_THEMES, ...loadCustomThemes()].find((x) => x.id === id) ?? BUILT_IN_THEMES[0])
}

// Home is the first page the user may open.
function Home() {
  const { can } = useApp()
  const to = Object.entries(ROUTE_PERMS).find(([path, perm]) => path !== '/ops' && can(perm))?.[0] ?? '/settings'
  return <Navigate to={to} replace />
}

// Guard shows a page only with its permission (the API checks it anyway).
function Guard({ path, children }: { path: string; children: ReactNode }) {
  const { can } = useApp()
  const perm = ROUTE_PERMS[path]
  return !perm || can(perm) ? children : <Home />
}

const page = (path: string, el: ReactNode) => <Route path={path} element={<Guard path={path.replace('/:id', '')}>{el}</Guard>} />

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter>
      <AppProvider>
        <Layout>
          <Routes>
            <Route path="/" element={<Home />} />
            {page('/ops', <OpsPage />)}
            {page('/incidents', <IncidentsPage />)}
            {page('/cmdb', <CmdbPage />)}
            {page('/heatmap', <HeatmapPage />)}
            {page('/cis', <CisPage />)}
            {page('/integrations', <IntegrationsPage />)}
            {page('/connectors', <ConnectorsPage />)}
            {page('/connectors/:id', <ConnectorEditorPage />)}
            {page('/events', <EventsPage />)}
            {page('/parse-errors', <ParseErrorsPage />)}
            {page('/rules', <RulesPage />)}
            {page('/maintenance', <MaintenancePage />)}
            {page('/selfcheck', <SelfCheckPage />)}
            {page('/audit', <AuditPage />)}
            {page('/notifications', <NotificationsPage />)}
            {page('/teams', <TeamsPage />)}
            {page('/users', <UsersPage />)}
            {page('/roles', <RolesPage />)}
            <Route path="/settings" element={<SettingsPage />} />
            <Route path="*" element={<Home />} />
          </Routes>
        </Layout>
      </AppProvider>
    </BrowserRouter>
  </StrictMode>,
)
