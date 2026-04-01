import { Routes, Route } from 'react-router-dom'
import { Layout } from './components/Layout'
import { PluginsPage } from './components/plugins/PluginsPage'
import { MetricsPage } from './components/metrics/MetricsPage'
import { DashboardPage } from './components/dashboard/DashboardPage'

function App() {
  return (
    <Routes>
      <Route path="/" element={<Layout />}>
        <Route index element={<PluginsPage />} />
        <Route path="plugins" element={<PluginsPage />} />
        <Route path="metrics" element={<MetricsPage />} />
        <Route path="dashboard" element={<DashboardPage />} />
      </Route>
    </Routes>
  )
}

export default App
