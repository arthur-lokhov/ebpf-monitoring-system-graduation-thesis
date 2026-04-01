import { useState, useEffect, useCallback } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer, BarChart, Bar } from 'recharts'
import { Plus, Edit, Trash2, Save, Grid3x3 } from 'lucide-react'
import { api, type Dashboard, type DashboardPanel, type MetricSeries } from '@/lib/api'

export function DashboardPage() {
  const [dashboard, setDashboard] = useState<Dashboard | null>(null)
  const [queryResults, setQueryResults] = useState<Record<string, MetricSeries[]>>({})
  const [editingPanel, setEditingPanel] = useState<DashboardPanel | null>(null)
  const [showPanelForm, setShowPanelForm] = useState(false)
  const [loading, setLoading] = useState(true)

  const loadDashboard = useCallback(async () => {
    try {
      const data = await api.getDashboard()
      setDashboard(data)
      
      // Load query results for all panels
      if (data?.config?.panels) {
        const results: Record<string, MetricSeries[]> = {}
        for (const panel of data.config.panels) {
          for (const query of panel.queries) {
            try {
              const result = await api.queryMetrics(query.expression)
              results[query.id] = result
            } catch (err) {
              console.error('Query failed:', err)
            }
          }
        }
        setQueryResults(results)
      }
    } catch (err) {
      console.error('Failed to load dashboard:', err)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadDashboard()
  }, [loadDashboard])

  const handleAddPanel = () => {
    setEditingPanel({
      id: `panel-${Date.now()}`,
      title: 'New Panel',
      type: 'graph',
      queries: [{
        id: `query-${Date.now()}`,
        expression: '',
        format: 'time_series',
      }],
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
      config: {
        ...dashboard.config,
        panels: updatedPanels,
      },
      updated_at: new Date().toISOString(),
    }

    try {
      const saved = await api.saveDashboard(updatedDashboard)
      setDashboard(saved)
      setShowPanelForm(false)
      setEditingPanel(null)
      
      // Reload query results
      const results: Record<string, MetricSeries[]> = {}
      for (const query of editingPanel.queries) {
        try {
          results[query.id] = await api.queryMetrics(query.expression)
        } catch (err) {
          console.error('Query failed:', err)
        }
      }
      setQueryResults(prev => ({ ...prev, ...results }))
    } catch (err) {
      console.error('Failed to save panel:', err)
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

    try {
      const saved = await api.saveDashboard(updatedDashboard)
      setDashboard(saved)
    } catch (err) {
      console.error('Failed to delete panel:', err)
    }
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
        { id: `query-${Date.now()}`, expression: '', format: 'time_series' },
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

  const renderPanel = (panel: DashboardPanel) => {
    const data = transformToChartData(queryResults[panel.queries[0]?.id] || [])

    return (
      <Card key={panel.id} className="relative">
        <CardHeader className="pb-2">
          <div className="flex items-center justify-between">
            <CardTitle className="text-lg">{panel.title}</CardTitle>
            <div className="flex gap-1">
              <Button variant="ghost" size="icon" onClick={() => handleEditPanel(panel)}>
                <Edit className="h-4 w-4" />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                onClick={() => handleDeletePanel(panel.id)}
              >
                <Trash2 className="h-4 w-4" />
              </Button>
            </div>
          </div>
          {panel.description && (
            <CardDescription>{panel.description}</CardDescription>
          )}
        </CardHeader>
        <CardContent>
          {panel.type === 'graph' && (
            <ResponsiveContainer width="100%" height={250}>
              <LineChart data={data}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="timestamp" />
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
                    />
                  ))}
              </LineChart>
            </ResponsiveContainer>
          )}

          {panel.type === 'stat' && (
            <div className="flex items-center justify-center h-32">
              <div className="text-4xl font-bold">
                {data.length > 0
                  ? (data[data.length - 1] as Record<string, unknown>)[
                      Object.keys(data[0] || {}).find(k => k !== 'timestamp') || ''
                    ] || 'N/A'
                  : 'N/A'}
              </div>
            </div>
          )}

          {panel.type === 'table' && (
            <div className="space-y-2">
              {data.slice(0, 10).map((row, i) => (
                <div key={i} className="flex justify-between text-sm">
                  <span>{(row as Record<string, unknown>).timestamp}</span>
                  <span className="font-mono">
                    {Object.entries(row)
                      .filter(([k]) => k !== 'timestamp')
                      .map(([_, v]) => v)
                      .join(', ')}
                  </span>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    )
  }

  const transformToChartData = (series: MetricSeries[]): unknown[] => {
    if (series.length === 0) return []

    const timestamps = new Set<string>()
    series.forEach(s => {
      s.points.forEach(p => {
        timestamps.add(new Date(p.timestamp).toISOString())
      })
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
        const seriesName = s.name
        point[seriesName] = seriesPoint?.value ?? null
      })
      
      return point
    })
  }

  if (loading) {
    return <div className="flex items-center justify-center h-64">Loading...</div>
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
        <Button onClick={handleAddPanel}>
          <Plus className="h-4 w-4 mr-2" />
          Add Panel
        </Button>
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
        <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
          {dashboard.config.panels.map(panel => renderPanel(panel))}
        </div>
      )}

      {/* Panel Editor Dialog */}
      {showPanelForm && editingPanel && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <Card className="w-full max-w-2xl max-h-[90vh] overflow-y-auto">
            <CardHeader>
              <CardTitle>Edit Panel</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div>
                <label className="text-sm font-medium">Title</label>
                <Input
                  value={editingPanel.title}
                  onChange={(e) =>
                    setEditingPanel({ ...editingPanel, title: e.target.value })
                  }
                />
              </div>
              <div>
                <label className="text-sm font-medium">Description</label>
                <Textarea
                  value={editingPanel.description || ''}
                  onChange={(e) =>
                    setEditingPanel({ ...editingPanel, description: e.target.value })
                  }
                />
              </div>
              <div>
                <label className="text-sm font-medium">Type</label>
                <select
                  value={editingPanel.type}
                  onChange={(e) =>
                    setEditingPanel({
                      ...editingPanel,
                      type: e.target.value as 'graph' | 'stat' | 'table' | 'heatmap',
                    })
                  }
                  className="w-full p-2 border rounded-md"
                >
                  <option value="graph">Line Chart</option>
                  <option value="stat">Stat</option>
                  <option value="table">Table</option>
                  <option value="heatmap">Heatmap</option>
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
                  {editingPanel.queries.map((query, i) => (
                    <div key={query.id} className="flex gap-2 items-start">
                      <Textarea
                        placeholder="Enter PromQL-like query"
                        value={query.expression}
                        onChange={(e) =>
                          handleUpdateQuery(i, 'expression', e.target.value)
                        }
                        className="flex-1 font-mono text-sm"
                        rows={2}
                      />
                      <Button
                        variant="outline"
                        size="icon"
                        onClick={() => handleRemoveQuery(i)}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  ))}
                </div>
              </div>
              <div className="flex gap-2 justify-end">
                <Button
                  variant="outline"
                  onClick={() => {
                    setShowPanelForm(false)
                    setEditingPanel(null)
                  }}
                >
                  Cancel
                </Button>
                <Button onClick={handleSavePanel}>
                  <Save className="h-4 w-4 mr-2" />
                  Save Panel
                </Button>
              </div>
            </CardContent>
          </Card>
        </div>
      )}
    </div>
  )
}
