import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { Layout } from './components/Layout'
import { AppProvider } from './context'
import { CmdbPage } from './pages/Cmdb'
import { ConnectorEditorPage } from './pages/ConnectorEditor'
import { ConnectorsPage } from './pages/Connectors'
import { IncidentsPage } from './pages/Incidents'
import { AuditPage, EventsPage, MaintenancePage, ParseErrorsPage, RolesPage, RulesPage, SelfCheckPage } from './pages/Misc'
import { OpsPage } from './pages/Ops'
import './styles.css'

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
            <Route path="/connectors" element={<ConnectorsPage />} />
            <Route path="/connectors/:id" element={<ConnectorEditorPage />} />
            <Route path="/events" element={<EventsPage />} />
            <Route path="/parse-errors" element={<ParseErrorsPage />} />
            <Route path="/rules" element={<RulesPage />} />
            <Route path="/maintenance" element={<MaintenancePage />} />
            <Route path="/selfcheck" element={<SelfCheckPage />} />
            <Route path="/audit" element={<AuditPage />} />
            <Route path="/roles" element={<RolesPage />} />
            <Route path="*" element={<Navigate to="/incidents" replace />} />
          </Routes>
        </Layout>
      </AppProvider>
    </BrowserRouter>
  </StrictMode>,
)
