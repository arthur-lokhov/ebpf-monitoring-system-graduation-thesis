import { useState, useEffect, useCallback } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer, BarChart, Bar } from 'recharts'
import { Plus, Edit, Trash2, Save, Grid3x3, AlertCircle, CheckCircle2 } from 'lucide-react'
import { api, type Dashboard, type DashboardPanel, type MetricSeries } from '@/lib/api'

const STORAGE_KEY = 'epbf_dashboard'

function genUUID(): string {
  return crypto.randomUUID?.() || `${Date.now().toString(16)}-${Math.random().toString(16).slice(2)}`
}

export function DashboardPage() {
  const [dashboard, setDashboard] = useState<Dashboard | null>(null)
  const [queryResults, setQueryResults] = useState<Record<string, MetricSeries[]>>({})
  const [editingPanel, setEditingPanel] = useState<DashboardPanel | null>(null)
  const [showPanelForm, setShowPanelForm] = useState(false)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [saveMsg, setSaveMsg] = useState<'idle' | 'saved' | 'backend_unavailable'>('idle')
  const [error, setError] = useState<string | null>(null)

  const loadDashboard = useCallback(async () => {
    try {
      // Try localStorage first
      try {
        const saved = localStorage.getItem(STORAGE_KEY)
        if (saved) {
          const parsed = JSON.parse(saved) as Dashboard
          setDashboard(parsed)
          // Load query results
          if (parsed.config?.panels) {
            await loadPanelQueries(parsed.config.panels)
          }
          setLoading(false)
          return
        }
      } catch { /* fallback */ }

      // Try backend
      try {
        let data = await api.getDashboard()

        if (!data) {
          const defaultDash: Dashboard = {
            id: genUUID(),
            name: 'Default Dashboard',
            description: 'Auto-created default dashboard',
            config: { version: 1, panels: [], layout: 'grid' },
            is_default: true,
            created_at: new Date().toISOString(),
            updated_at: new Date().toISOString(),
          }
          try {
            data = await api.saveDashboard(defaultDash)
          } catch {
            data = defaultDash
          }
        }

        setDashboard(data)
        if (data?.config?.panels) {
          await loadPanelQueries(data.config.panels)
        }
      } catch {
        // Backend unavailable — create local dashboard
        const localDash: Dashboard = {
          id: genUUID(),
          name: 'Local Dashboard',
          description: 'Created locally (backend unavailable)',
          config: { version: 1, panels: [], layout: 'grid' },
          is_default: true,
          created_at: new Date().toISOString(),
          updated_at: new Date().toISOString(),
        }
        setDashboard(localDash)
        setSaveMsg('backend_unavailable')
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load dashboard')
    } finally {
      setLoading(false)
    }
  }, [])

  const loadPanelQueries = async (panels: DashboardPanel[]) => {
    const results: Record<string, MetricSeries[]> = {}
    for (const panel of panels) {
      for (const q of panel.queries) {
        if (!q.expression?.trim()) continue
        try {
          const now = Math.floor(Date.now() / 1000)
          // Use Prometheus directly for range queries
          const res = await fetch(
            `${import.meta.env.VITE_PROMETHEUS_URL || 'http://localhost:9090'}/api/v1/query_range?query=${encodeURIComponent(q.expression)}&start=${now - 300}&end=${now}&step=15s`
          )
          const json = await res.json()
          const promResults = json.data?.result || []
          results[q.id] = promResults.map((r: { metric: Record<string, string>; values?: [number, string][] }) => ({
            name: Object.entries(r.metric).filter(([k]) => k !== '__name__').map(([k, v]) => `${k}="${v}"`).join(','),
            labels: Object.fromEntries(Object.entries(r.metric).filter(([k]) => k !== '__name__')),
            points: (r.values || []).map(([ts, val]) => ({
              timestamp: new Date(ts * 1000).toISOString(),
              value: parseFloat(val),
            })),
          }))
        } catch { /* skip failed queries */ }
      }
    }
    setQueryResults(results)
  }

  useEffect(() => {
    loadDashboard()
  }, [loadDashboard])

  const persistToLocalStorage = (dash: Dashboard) => {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(dash))
    } catch { /* quota exceeded */ }
  }

  const saveDashboard = async (dash: Dashboard) => {
    setSaving(true)
    try {
      let saved: Dashboard
      try {
        saved = await api.saveDashboard(dash)
      } catch {
        // Backend unavailable — save locally
        saved = dash
        setSaveMsg('backend_unavailable')
      }
      setDashboard(saved)
      persistToLocalStorage(saved)
      setSaveMsg('saved')
      setTimeout(() => setSaveMsg('idle'), 3000)
    } finally {
      setSaving(false)
    }
  }

  const handleAddPanel = () => {
    setEditingPanel({
      id: genUUID(),
      title: 'New Panel',
      type: 'graph',
      queries: [{ id: genUUID(), expression: '', format: 'time_series' }],
      gridPos: { x: 0, y: 0, w: 6, h: 4 },
    })
    setShowPanelForm(true)
  }

  const handleSavePanel = async () => {
    if (!editingPanel || !dashboard) return

    const updatedPanels = dashboard.config.panels.some(p => p.id === editingPanel.id)
      ? dashboard.config.panels.map(p => p.id === editingPanel.id ? editingPanel : p)
      : [...dashboard.config.panels, editingPanel]

    const updatedDashboard: Dashboard = {
      ...dashboard,
      config: { ...dashboard.config, panels: updatedPanels },
      updated_at: new Date().toISOString(),
    }

    await saveDashboard(updatedDashboard)
    setShowPanelForm(false)
    setEditingPanel(null)

    // Reload query results for the saved panel
    if (editingPanel.queries.length > 0) {
      await loadPanelQueries([editingPanel])
    }
  }

  const handleDeletePanel = async (panelId: string) => {
    if (!dashboard) return

    const updatedDashboard: Dashboard = {
      ...dashboard,
      config: {
        ...dashboard.config,
        panels: dashboard.config.panels.filter(p => p.id !== panelId),
      },
      updated_at: new Date().toISOString(),
    }

    await saveDashboard(updatedDashboard)
  }

  const handleEditPanel = (panel: DashboardPanel) => {
    setEditingPanel({ ...panel })
    setShowPanelForm(true)
  }

  const handleUpdateQuery = (queryIndex: number, field: string, value: string) => {
    if (!editingPanel) return
    setEditingPanel({
      ...editingPanel,
      queries: editingPanel.queries.map((q, i) =>
        i === queryIndex ? { ...q, [field]: value } : q
      ),
    })
  }

  const handleAddQuery = () => {
    if (!editingPanel) return
    setEditingPanel({
      ...editingPanel,
      queries: [
        ...editingPanel.queries,
        { id: genUUID(), expression: '', format: 'time_series' },
      ],
    })
  }

  const handleRemoveQuery = (queryIndex: number) => {
    if (!editingPanel) return
    setEditingPanel({
      ...editingPanel,
      queries: editingPanel.queries.filter((_, i) => i !== queryIndex),
    })
  }

  const transformToChartData = (series: MetricSeries[]): Record<string, unknown>[] => {
    if (series.length === 0) return []

    const timestamps = new Set<string>()
    series.forEach(s => {
      s.points.forEach(p => timestamps.add(new Date(p.timestamp).toISOString()))
    })

    const sortedTimestamps = Array.from(timestamps).sort()

    return sortedTimestamps.map(timestamp => {
      const point: Record<string, unknown> = {
        timestamp: new Date(timestamp).toLocaleTimeString(),
      }

      series.forEach(s => {
        const seriesPoint = s.points.find(p =>
          new Date(p.timestamp).toISOString() === timestamp
        )
        const seriesName = s.name || Object.entries(s.labels).map(([k, v]) => `${k}="${v}"`).join(',') || 'value'
        point[seriesName] = seriesPoint?.value ?? null
      })

      return point
    })
  }

  const renderPanel = (panel: DashboardPanel) => {
    const allSeries: MetricSeries[] = []
    panel.queries.forEach(q => {
      const results = queryResults[q.id]
      if (results) allSeries.push(...results)
    })

    const data = transformToChartData(allSeries)

    const typeLabel: Record<string, string> = {
      graph: 'Line Chart',
      stat: 'Stat',
      table: 'Table',
      heatmap: 'Bar Chart',
    }

    return (
      <Card key={panel.id} className="relative">
        <CardHeader className="pb-2">
          <div className="flex items-center justify-between">
            <div>
              <CardTitle className="text-lg">{panel.title}</CardTitle>
              {panel.description && <CardDescription>{panel.description}</CardDescription>}
            </div>
            <div className="flex gap-1">
              <Button variant="ghost" size="icon" onClick={() => handleEditPanel(panel)}>
                <Edit className="h-4 w-4" />
              </Button>
              <Button variant="ghost" size="icon" onClick={() => handleDeletePanel(panel.id)}>
                <Trash2 className="h-4 w-4" />
              </Button>
            </div>
          </div>
          <Badge variant="outline" className="w-fit">{typeLabel[panel.type] || panel.type}</Badge>
        </CardHeader>
        <CardContent>
          {panel.type === 'graph' && (
            data.length > 0 ? (
              <ResponsiveContainer width="100%" height={250}>
                <LineChart data={data}>
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis dataKey="timestamp" tick={{ fontSize: 11 }} />
                  <YAxis />
                  <Tooltip />
                  <Legend />
                  {Object.keys(data[0] || {})
                    .filter(key => key !== 'timestamp')
                    .map((key, i) => (
                      <Line
                        key={key}
                        type="monotone"
                        dataKey={key}
                        stroke={`hsl(${i * 60}, 70%, 50%)`}
                        dot={false}
                        strokeWidth={2}
                      />
                    ))}
                </LineChart>
              </ResponsiveContainer>
            ) : (
              <p className="text-muted-foreground text-center py-8 text-sm">
                No data — check your query and that Prometheus is running
              </p>
            )
          )}

          {panel.type === 'stat' && (
            <div className="flex items-center justify-center h-32">
              {data.length > 0 ? (
                <div className="text-center">
                  <div className="text-4xl font-bold">
                    {String(
                      (data[data.length - 1] as Record<string, unknown>)[
                        Object.keys(data[0] || {}).find(k => k !== 'timestamp') || ''
                      ] || 'N/A'
                    )}
                  </div>
                  <p className="text-sm text-muted-foreground mt-1">Latest value</p>
                </div>
              ) : (
                <p className="text-muted-foreground">No data</p>
              )}
            </div>
          )}

          {panel.type === 'table' && (
            data.length > 0 ? (
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b">
                      <th className="text-left p-2">Timestamp</th>
                      {Object.keys(data[0] || {})
                        .filter(k => k !== 'timestamp')
                        .map(k => <th key={k} className="text-left p-2">{k}</th>)}
                    </tr>
                  </thead>
                  <tbody>
                    {data.slice(0, 20).map((row, i) => (
                      <tr key={i} className="border-b">
                        <td className="p-2 font-mono text-xs">{String((row as Record<string, unknown>).timestamp)}</td>
                        {Object.entries(row as Record<string, unknown>)
                          .filter(([k]) => k !== 'timestamp')
                          .map(([k, v]) => (
                            <td key={k} className="p-2 font-mono text-xs">{String(v)}</td>
                          ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : (
              <p className="text-muted-foreground text-center py-8 text-sm">No data</p>
            )
          )}

          {panel.type === 'heatmap' && (
            data.length > 0 ? (
              <ResponsiveContainer width="100%" height={250}>
                <BarChart data={data}>
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis dataKey="timestamp" tick={{ fontSize: 11 }} />
                  <YAxis />
                  <Tooltip />
                  <Legend />
                  {Object.keys(data[0] || {})
                    .filter(key => key !== 'timestamp')
                    .map((key, i) => (
                      <Bar
                        key={key}
                        dataKey={key}
                        fill={`hsl(${i * 60}, 70%, 50%)`}
                      />
                    ))}
                </BarChart>
              </ResponsiveContainer>
            ) : (
              <p className="text-muted-foreground text-center py-8 text-sm">No data</p>
            )
          )}
        </CardContent>
      </Card>
    )
  }

  if (loading) {
    return <div className="flex items-center justify-center h-64">Loading...</div>
  }

  if (error) {
    return <div className="text-destructive text-center py-8">{error}</div>
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Dashboard</h1>
          <p className="text-muted-foreground">
            Visualize your metrics with customizable panels
          </p>
        </div>
        <div className="flex items-center gap-2">
          {saveMsg === 'saved' && (
            <span className="text-sm text-green-600 flex items-center gap-1">
              <CheckCircle2 className="h-3 w-3" /> Saved
            </span>
          )}
          {saveMsg === 'backend_unavailable' && (
            <span className="text-sm text-amber-600 flex items-center gap-1">
              <AlertCircle className="h-3 w-3" /> Saved locally (backend unavailable)
            </span>
          )}
          <Button onClick={handleAddPanel}>
            <Plus className="h-4 w-4 mr-2" />
            Add Panel
          </Button>
        </div>
      </div>

      {!dashboard?.config?.panels || dashboard.config.panels.length === 0 ? (
        <Card>
          <CardContent className="flex flex-col items-center justify-center py-16">
            <Grid3x3 className="h-16 w-16 text-muted-foreground mb-4" />
            <h3 className="text-lg font-medium">No panels yet</h3>
            <p className="text-muted-foreground mb-4">
              Add your first panel to start visualizing metrics
            </p>
            <Button onClick={handleAddPanel}>
              <Plus className="h-4 w-4 mr-2" />
              Add Panel
            </Button>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-6 md:grid-cols-2">
          {dashboard.config.panels.map(panel => renderPanel(panel))}
        </div>
      )}

      {/* Panel Editor Dialog */}
      {showPanelForm && editingPanel && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <Card className="w-full max-w-2xl max-h-[90vh] overflow-y-auto">
            <CardHeader>
              <CardTitle>{dashboard?.config.panels?.some(p => p.id === editingPanel.id) ? 'Edit Panel' : 'New Panel'}</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div>
                <label className="text-sm font-medium">Title</label>
                <Input
                  value={editingPanel.title}
                  onChange={(e) => setEditingPanel({ ...editingPanel, title: e.target.value })}
                />
              </div>
              <div>
                <label className="text-sm font-medium">Description</label>
                <Textarea
                  value={editingPanel.description || ''}
                  onChange={(e) => setEditingPanel({ ...editingPanel, description: e.target.value })}
                />
              </div>
              <div>
                <label className="text-sm font-medium">Type</label>
                <select
                  value={editingPanel.type}
                  onChange={(e) => setEditingPanel({ ...editingPanel, type: e.target.value as 'graph' | 'stat' | 'table' | 'heatmap' })}
                  className="w-full p-2 border rounded-md mt-1"
                >
                  <option value="graph">Line Chart</option>
                  <option value="stat">Stat</option>
                  <option value="table">Table</option>
                  <option value="heatmap">Bar Chart</option>
                </select>
              </div>
              <div>
                <div className="flex items-center justify-between mb-2">
                  <label className="text-sm font-medium">Queries</label>
                  <Button variant="outline" size="sm" onClick={handleAddQuery}>
                    <Plus className="h-3 w-3 mr-1" />
                    Add Query
                  </Button>
                </div>
                <div className="space-y-2">
                  {editingPanel.queries.map((q, i) => (
                    <div key={q.id} className="flex gap-2 items-start">
                      <Textarea
                        placeholder="rate(epbf_metric[1m])"
                        value={q.expression}
                        onChange={(e) => handleUpdateQuery(i, 'expression', e.target.value)}
                        className="flex-1 font-mono text-sm"
                        rows={2}
                      />
                      <Button variant="outline" size="icon" onClick={() => handleRemoveQuery(i)}>
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  ))}
                </div>
              </div>
              <div className="flex gap-2 justify-end pt-2">
                <Button variant="outline" onClick={() => { setShowPanelForm(false); setEditingPanel(null) }}>
                  Cancel
                </Button>
                <Button onClick={handleSavePanel} disabled={saving}>
                  <Save className="h-4 w-4 mr-2" />
                  {saving ? 'Saving...' : 'Save Panel'}
                </Button>
              </div>
            </CardContent>
          </Card>
        </div>
      )}
    </div>
  )
}
