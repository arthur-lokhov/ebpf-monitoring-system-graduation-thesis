import { useState, useEffect, useCallback } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts'
import { Search, Filter, Play, Plus, Trash2, HelpCircle, CheckCircle2, AlertCircle } from 'lucide-react'
import { api, prometheus, type MetricInfo, type Filter as FilterType, type MetricSeries } from '@/lib/api'

const STORAGE_KEY_QUERY = 'epbf_metrics_query'
const STORAGE_KEY_RESULT = 'epbf_metrics_result'
const STORAGE_KEY_CHART = 'epbf_metrics_chart'
const STORAGE_KEY_FILTERS = 'epbf_metrics_filters'

export function MetricsPage() {
  const [metricNames, setMetricNames] = useState<string[]>([])
  const [selectedMetric, setSelectedMetric] = useState<string | null>(null)
  const [metricInfo, setMetricInfo] = useState<MetricInfo | null>(null)
  const [metricError, setMetricError] = useState<string | null>(null)
  const [filters, setFilters] = useState<FilterType[]>([])
  const [query, setQuery] = useState(() => {
    try { return localStorage.getItem(STORAGE_KEY_QUERY) || '' } catch { return '' }
  })
  const [queryResult, setQueryResult] = useState<MetricSeries[] | null>(() => {
    try {
      const saved = localStorage.getItem(STORAGE_KEY_RESULT)
      return saved ? JSON.parse(saved) : null
    } catch { return null }
  })
  const [chartData, setChartData] = useState<unknown[]>(() => {
    try {
      const saved = localStorage.getItem(STORAGE_KEY_CHART)
      return saved ? JSON.parse(saved) : []
    } catch { return [] }
  })
  const [loading, setLoading] = useState(true)
  const [filterName, setFilterName] = useState('')
  const [filterExpression, setFilterExpression] = useState('')
  const [showFilterForm, setShowFilterForm] = useState(false)
  const [showHelp, setShowHelp] = useState(false)
  const [saveStatus, setSaveStatus] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle')

  const loadData = useCallback(async () => {
    try {
      // Try loading filters from backend first
      try {
        const backendFilters = await api.getFilters()
        setFilters(backendFilters)
        try { localStorage.setItem(STORAGE_KEY_FILTERS, JSON.stringify(backendFilters)) } catch {}
      } catch {
        // Fallback to localStorage
        try {
          const saved = localStorage.getItem(STORAGE_KEY_FILTERS)
          if (saved) setFilters(JSON.parse(saved))
        } catch {}
      }

      // Load metric names from Prometheus
      const names = await prometheus.getMetricNames()
      setMetricNames(names.filter((n: string) => n.startsWith('epbf_')))
    } catch (err) {
      console.error('Failed to load data:', err)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadData()
  }, [loadData])

  const handleSelectMetric = async (name: string) => {
    setSelectedMetric(name)
    setMetricError(null)
    try {
      const results = await prometheus.query(name)
      if (results.length > 0) {
        const first = results[0]
        setMetricInfo({
          name,
          label_names: Object.keys(first.metric).filter(k => k !== '__name__'),
          label_values: {},
          latest_value: parseFloat(first.value[1]),
          latest_time: new Date(first.value[0] * 1000).toISOString(),
          total_points: results.length,
        })
      } else {
        // Metric exists but has no current value
        setMetricInfo({
          name,
          label_names: [],
          label_values: {},
          latest_value: 0,
          latest_time: new Date().toISOString(),
          total_points: 0,
        })
      }
    } catch (err) {
      console.error('Failed to get metric info:', err)
      setMetricError(err instanceof Error ? err.message : 'Failed to query metric')
      setMetricInfo(null)
    }
  }

  const handleExecuteQuery = async () => {
    if (!query.trim()) return
    try {
      const now = Math.floor(Date.now() / 1000)
      const results = await prometheus.queryRange(query, now - 300, now, '15s')

      const series: MetricSeries[] = results.map(r => ({
        name: Object.entries(r.metric).filter(([k]) => k !== '__name__').map(([k,v]) => `${k}="${v}"`).join(','),
        labels: Object.fromEntries(Object.entries(r.metric).filter(([k]) => k !== '__name__')),
        points: (r.values || []).map(([ts, val]) => ({
          timestamp: new Date(ts * 1000).toISOString(),
          value: parseFloat(val),
        })),
      }))

      setQueryResult(series)

      if (series.length > 0) {
        const transformed = transformToChartData(series)
        setChartData(transformed)
      }

      // Persist to localStorage
      try {
        localStorage.setItem(STORAGE_KEY_QUERY, query)
        localStorage.setItem(STORAGE_KEY_RESULT, JSON.stringify(series))
        localStorage.setItem(STORAGE_KEY_CHART, JSON.stringify(series.length > 0 ? transformToChartData(series) : []))
      } catch { /* quota exceeded */ }
    } catch (err) {
      console.error('Query failed:', err)
      setQueryResult(null)
      setChartData([])
    }
  }

  const handleCreateFilter = async () => {
    if (!filterName.trim() || !filterExpression.trim()) return
    setSaveStatus('saving')
    try {
      const newFilter = await api.createFilter({
        name: filterName,
        expression: filterExpression,
        is_default: false,
      })
      const updated = [...filters, newFilter]
      setFilters(updated)
      try { localStorage.setItem(STORAGE_KEY_FILTERS, JSON.stringify(updated)) } catch {}
      setFilterName('')
      setFilterExpression('')
      setShowFilterForm(false)
      setSaveStatus('saved')
      setTimeout(() => setSaveStatus('idle'), 2000)
    } catch (err) {
      console.error('Failed to create filter:', err)
      // Fallback: save to localStorage only
      const localFilter: FilterType = {
        id: `local-${Date.now()}`,
        name: filterName,
        expression: filterExpression,
        is_default: false,
        created_at: new Date().toISOString(),
      }
      const updated = [...filters, localFilter]
      setFilters(updated)
      try { localStorage.setItem(STORAGE_KEY_FILTERS, JSON.stringify(updated)) } catch {}
      setFilterName('')
      setFilterExpression('')
      setShowFilterForm(false)
      setSaveStatus('saved')
      setTimeout(() => setSaveStatus('idle'), 2000)
    }
  }

  const handleDeleteFilter = async (id: string) => {
    try {
      await api.deleteFilter(id)
    } catch { /* may be local filter — just remove from state */ }
    const updated = filters.filter(f => f.id !== id)
    setFilters(updated)
    try { localStorage.setItem(STORAGE_KEY_FILTERS, JSON.stringify(updated)) } catch {}
  }

  const handleApplyFilter = async (expression: string) => {
    setQuery(expression)
    try {
      const now = Math.floor(Date.now() / 1000)
      const results = await prometheus.queryRange(expression, now - 300, now, '15s')
      const series: MetricSeries[] = results.map(r => ({
        name: Object.entries(r.metric).filter(([k]) => k !== '__name__').map(([k,v]) => `${k}="${v}"`).join(','),
        labels: Object.fromEntries(Object.entries(r.metric).filter(([k]) => k !== '__name__')),
        points: (r.values || []).map(([ts, val]) => ({
          timestamp: new Date(ts * 1000).toISOString(),
          value: parseFloat(val),
        })),
      }))
      setQueryResult(series)
      if (series.length > 0) {
        const transformed = transformToChartData(series)
        setChartData(transformed)
      }
    } catch (err) {
      console.error('Filter execution failed:', err)
      setQueryResult(null)
      setChartData([])
    }
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
        const seriesName = s.name + (Object.keys(s.labels).length > 0
          ? ` {${Object.entries(s.labels).map(([k, v]) => `${k}="${v}"`).join(', ')}}`
          : '')
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
          <h1 className="text-3xl font-bold tracking-tight">Metrics</h1>
          <p className="text-muted-foreground">
            Browse and query metrics from your plugins
          </p>
        </div>
        <Button variant="outline" onClick={() => setShowHelp(true)}>
          <HelpCircle className="h-4 w-4 mr-2" />
          Query Help
        </Button>
      </div>

      {/* Help Modal */}
      {showHelp && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={() => setShowHelp(false)}>
          <div className="bg-background w-full max-w-3xl max-h-[80vh] overflow-y-auto rounded-lg shadow-lg" onClick={(e) => e.stopPropagation()}>
            <div className="sticky top-0 bg-background border-b p-4 flex items-center justify-between">
              <h2 className="text-xl font-bold">Query Language Guide</h2>
              <Button variant="ghost" size="icon" onClick={() => setShowHelp(false)}>
                <span className="text-2xl">&times;</span>
              </Button>
            </div>
            <div className="p-6 space-y-4">
              <p className="text-muted-foreground">PromQL-like query syntax for filtering and aggregating metrics</p>

              <Card>
                <CardHeader><CardTitle className="text-lg">Basic Queries</CardTitle></CardHeader>
                <CardContent className="space-y-2">
                  <div>
                    <p className="font-mono text-sm bg-muted p-2 rounded">rate(metric_name[1m])</p>
                    <p className="text-sm text-muted-foreground">Calculate per-second rate over 1 minute</p>
                  </div>
                  <div>
                    <p className="font-mono text-sm bg-muted p-2 rounded">sum(metric_name)</p>
                    <p className="text-sm text-muted-foreground">Sum all values of a metric</p>
                  </div>
                </CardContent>
              </Card>

              <Card>
                <CardHeader><CardTitle className="text-lg">Grouping with by</CardTitle></CardHeader>
                <CardContent className="space-y-2">
                  <div>
                    <p className="font-mono text-sm bg-muted p-2 rounded">sum by (label1, label2) (metric_name)</p>
                    <p className="text-sm text-muted-foreground">Group results by specified labels</p>
                  </div>
                </CardContent>
              </Card>

              <Card>
                <CardHeader><CardTitle className="text-lg">Common Functions</CardTitle></CardHeader>
                <CardContent className="space-y-2">
                  <div>
                    <p className="font-mono text-sm bg-muted p-2 rounded">rate(counter[1m])</p>
                    <p className="text-sm text-muted-foreground">Per-second rate for counters</p>
                  </div>
                  <div>
                    <p className="font-mono text-sm bg-muted p-2 rounded">histogram_quantile(0.95, metric_bucket)</p>
                    <p className="text-sm text-muted-foreground">95th percentile from histogram</p>
                  </div>
                </CardContent>
              </Card>
            </div>
          </div>
        </div>
      )}

      {/* Query Editor */}
      <Card>
        <CardHeader>
          <CardTitle>Query Editor</CardTitle>
          <CardDescription>
            Execute PromQL-like queries to filter and aggregate metrics
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            <Textarea
              placeholder="Enter query (e.g., rate(tcp_connections_total[1m]))"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              className="font-mono text-sm"
              rows={3}
            />
            <div className="flex gap-2">
              <Button onClick={handleExecuteQuery}>
                <Play className="h-4 w-4 mr-2" />
                Execute
              </Button>
              <Button
                variant="outline"
                onClick={() => {
                  setShowFilterForm(!showFilterForm)
                  setFilterExpression(query)
                }}
              >
                <Filter className="h-4 w-4 mr-2" />
                Save as Filter
              </Button>
            </div>

            {showFilterForm && (
              <div className="space-y-3 p-4 border rounded-md">
                <div className="flex gap-2">
                  <Input
                    placeholder="Filter name"
                    value={filterName}
                    onChange={(e) => setFilterName(e.target.value)}
                    className="flex-1"
                  />
                  <Input
                    placeholder="Expression (pre-filled from query)"
                    value={filterExpression}
                    onChange={(e) => setFilterExpression(e.target.value)}
                    className="flex-1"
                  />
                </div>
                <div className="flex gap-2">
                  <Button onClick={handleCreateFilter} disabled={saveStatus === 'saving'}>
                    {saveStatus === 'saving' ? (
                      'Saving...'
                    ) : saveStatus === 'saved' ? (
                      <><CheckCircle2 className="h-4 w-4 mr-2" /> Saved</>
                    ) : (
                      <><Plus className="h-4 w-4 mr-2" /> Save</>
                    )}
                  </Button>
                  {saveStatus === 'error' && (
                    <span className="text-sm text-destructive flex items-center gap-1">
                      <AlertCircle className="h-3 w-3" /> Saved locally (backend unavailable)
                    </span>
                  )}
                </div>
              </div>
            )}
          </div>
        </CardContent>
      </Card>

      {/* Saved Filters */}
      {filters.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>Saved Filters</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="flex flex-wrap gap-2">
              {filters.map((filter) => (
                <div
                  key={filter.id}
                  className="flex items-center gap-2 p-2 border rounded-md"
                >
                  <Badge variant="secondary">{filter.name}</Badge>
                  <code className="text-xs max-w-[300px] truncate">{filter.expression}</code>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => handleApplyFilter(filter.expression)}
                  >
                    <Play className="h-3 w-3" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => handleDeleteFilter(filter.id)}
                  >
                    <Trash2 className="h-3 w-3" />
                  </Button>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Query Results Chart */}
      {queryResult && chartData.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>Query Results</CardTitle>
          </CardHeader>
          <CardContent>
            <ResponsiveContainer width="100%" height={400}>
              <LineChart data={chartData}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="timestamp" />
                <YAxis />
                <Tooltip />
                <Legend />
                {Object.keys(chartData[0] || {})
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
          </CardContent>
        </Card>
      )}

      {/* Metrics List */}
      <div className="grid gap-6 md:grid-cols-2">
        {/* Available Metrics — scrollable */}
        <Card>
          <CardHeader>
            <CardTitle>Available Metrics</CardTitle>
            <CardDescription>
              {metricNames.length} metric{metricNames.length !== 1 ? 's' : ''} available
            </CardDescription>
          </CardHeader>
          <CardContent>
            {metricNames.length === 0 ? (
              <p className="text-muted-foreground text-center py-8">
                No metrics available. Add and enable plugins to collect metrics.
              </p>
            ) : (
              <div className="space-y-2 max-h-[500px] overflow-y-auto pr-2">
                {metricNames.map((name) => (
                  <div
                    key={name}
                    className={`flex items-center justify-between p-2 rounded-md cursor-pointer transition-colors ${
                      selectedMetric === name
                        ? 'bg-accent'
                        : 'hover:bg-accent/50'
                    }`}
                    onClick={() => handleSelectMetric(name)}
                  >
                    <code className="text-sm truncate">{name}</code>
                    <Search className="h-4 w-4 text-muted-foreground shrink-0 ml-2" />
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>

        {/* Metric Details */}
        <Card>
          <CardHeader>
            <CardTitle>
              {metricInfo ? metricInfo.name : 'Metric Details'}
            </CardTitle>
          </CardHeader>
          <CardContent>
            {metricError ? (
              <div className="space-y-2">
                <p className="text-destructive text-sm">{metricError}</p>
                <p className="text-muted-foreground text-sm">
                  Prometheus may be unavailable. Check that it is running on port 9090.
                </p>
              </div>
            ) : metricInfo ? (
              <div className="space-y-4">
                <div>
                  <h4 className="text-sm font-medium">Labels</h4>
                  <div className="flex flex-wrap gap-2 mt-2">
                    {metricInfo.label_names.length === 0 ? (
                      <span className="text-muted-foreground text-sm">No labels</span>
                    ) : (
                      metricInfo.label_names.map((label) => (
                        <Badge key={label} variant="outline">{label}</Badge>
                      ))
                    )}
                  </div>
                </div>
                <div>
                  <h4 className="text-sm font-medium">Latest Value</h4>
                  <p className="text-2xl font-bold mt-1">
                    {metricInfo.latest_value.toLocaleString()}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {metricInfo.total_points > 0
                      ? `at ${new Date(metricInfo.latest_time).toLocaleString()}`
                      : 'No data points yet'}
                  </p>
                </div>
                {metricInfo.total_points > 0 && (
                  <div>
                    <h4 className="text-sm font-medium">Total Data Points</h4>
                    <p className="text-lg">{metricInfo.total_points}</p>
                  </div>
                )}
              </div>
            ) : (
              <p className="text-muted-foreground text-center py-8">
                Select a metric to view details
              </p>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
