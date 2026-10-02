import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { Layout } from './components/Layout'
import { AppProvider } from './context'
import { CmdbPage } from './pages/Cmdb'
import { ConnectorEditorPage } from './pages/ConnectorEditor'
import { ConnectorsPage } from './pages/Connectors'
import { HeatmapPage } from './pages/Heatmap'
import { IncidentsPage } from './pages/Incidents'
import { AuditPage, EventsPage, MaintenancePage, ParseErrorsPage, RolesPage, RulesPage, SelfCheckPage } from './pages/Misc'
import { OpsPage } from './pages/Ops'
import { SettingsPage } from './pages/Settings'
import { applyTheme, BUILT_IN_THEMES, loadActiveTheme, loadCustomThemes } from './theme'
import './styles.css'

// Paint the saved theme before the first render so the page does not flash.
{
  const id = loadActiveTheme()
  applyTheme([...BUILT_IN_THEMES, ...loadCustomThemes()].find((x) => x.id === id) ?? BUILT_IN_THEMES[0])
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter>
      <AppProvider>
        <Layout>
          <Routes>
            <Route path="/" element={<Navigate to="/incidents" replace />} />
            <Route path="/ops" element={<OpsPage />} />
            <Route path="/incidents" element={<IncidentsPage />} />
            <Route path="/cmdb" element={<CmdbPage />} />
            <Route path="/heatmap" element={<HeatmapPage />} />
            <Route path="/connectors" element={<ConnectorsPage />} />
            <Route path="/connectors/:id" element={<ConnectorEditorPage />} />
            <Route path="/events" element={<EventsPage />} />
            <Route path="/parse-errors" element={<ParseErrorsPage />} />
            <Route path="/rules" element={<RulesPage />} />
            <Route path="/maintenance" element={<MaintenancePage />} />
            <Route path="/selfcheck" element={<SelfCheckPage />} />
            <Route path="/audit" element={<AuditPage />} />
            <Route path="/roles" element={<RolesPage />} />
            <Route path="/settings" element={<SettingsPage />} />
            <Route path="*" element={<Navigate to="/incidents" replace />} />
          </Routes>
        </Layout>
      </AppProvider>
    </BrowserRouter>
  </StrictMode>,
)
