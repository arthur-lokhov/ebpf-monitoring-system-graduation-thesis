import { useState, useEffect, useCallback } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts'
import { Search, Filter, Play, Plus, Trash2 } from 'lucide-react'
import { api, type MetricSample, type MetricInfo, type Filter as FilterType, type MetricSeries } from '@/lib/api'

export function MetricsPage() {
  const [metrics, setMetrics] = useState<MetricSample[]>([])
  const [metricNames, setMetricNames] = useState<string[]>([])
  const [selectedMetric, setSelectedMetric] = useState<string | null>(null)
  const [metricInfo, setMetricInfo] = useState<MetricInfo | null>(null)
  const [filters, setFilters] = useState<FilterType[]>([])
  const [query, setQuery] = useState('')
  const [queryResult, setQueryResult] = useState<MetricSeries[] | null>(null)
  const [chartData, setChartData] = useState<unknown[]>([])
  const [loading, setLoading] = useState(true)
  const [filterName, setFilterName] = useState('')
  const [filterExpression, setFilterExpression] = useState('')
  const [showFilterForm, setShowFilterForm] = useState(false)

  const loadData = useCallback(async () => {
    try {
      const [metricsData, namesData, filtersData] = await Promise.all([
        api.getMetrics(),
        api.getMetricNames(),
        api.getFilters(),
      ])
      setMetrics(metricsData)
      setMetricNames(namesData)
      setFilters(filtersData)
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
    try {
      const info = await api.getMetricByName(name)
      setMetricInfo(info)
    } catch (err) {
      console.error('Failed to get metric info:', err)
    }
  }

  const handleExecuteQuery = async () => {
    if (!query.trim()) return
    try {
      const result = await api.queryMetrics(query)
      setQueryResult(result)
      
      // Transform data for chart
      if (result.length > 0) {
        const transformed = transformToChartData(result)
        setChartData(transformed)
      }
    } catch (err) {
      console.error('Query failed:', err)
      setQueryResult(null)
      setChartData([])
    }
  }

  const handleCreateFilter = async () => {
    if (!filterName.trim() || !filterExpression.trim()) return
    try {
      const newFilter = await api.createFilter({
        name: filterName,
        expression: filterExpression,
        is_default: false,
      })
      setFilters([...filters, newFilter])
      setFilterName('')
      setFilterExpression('')
      setShowFilterForm(false)
    } catch (err) {
      console.error('Failed to create filter:', err)
    }
  }

  const handleDeleteFilter = async (id: string) => {
    try {
      await api.deleteFilter(id)
      setFilters(filters.filter(f => f.id !== id))
    } catch (err) {
      console.error('Failed to delete filter:', err)
    }
  }

  const handleApplyFilter = (expression: string) => {
    setQuery(expression)
  }

  const transformToChartData = (series: MetricSeries[]): unknown[] => {
    if (series.length === 0) return []

    // Collect all timestamps
    const timestamps = new Set<string>()
    series.forEach(s => {
      s.points.forEach(p => {
        timestamps.add(new Date(p.timestamp).toISOString())
      })
    })

    const sortedTimestamps = Array.from(timestamps).sort()

    // Create chart data points
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
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Metrics</h1>
        <p className="text-muted-foreground">
          Browse and query metrics from your plugins
        </p>
      </div>

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
              placeholder="Enter query (e.g., rate(tcp_connections_total[1m]), sum by (interface) (bytes_total))"
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
                onClick={() => setShowFilterForm(!showFilterForm)}
              >
                <Filter className="h-4 w-4 mr-2" />
                Save as Filter
              </Button>
            </div>

            {showFilterForm && (
              <div className="flex gap-2 p-4 border rounded-md">
                <Input
                  placeholder="Filter name"
                  value={filterName}
                  onChange={(e) => setFilterName(e.target.value)}
                  className="flex-1"
                />
                <Input
                  placeholder="Expression"
                  value={filterExpression}
                  onChange={(e) => setFilterExpression(e.target.value)}
                  className="flex-1"
                />
                <Button onClick={handleCreateFilter}>
                  <Plus className="h-4 w-4 mr-2" />
                  Save
                </Button>
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
                  <code className="text-xs">{filter.expression}</code>
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
        {/* Available Metrics */}
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
              <div className="space-y-2">
                {metricNames.map((name) => (
                  <div
                    key={name}
                    className={`flex items-center justify-between p-2 rounded-md cursor-pointer ${
                      selectedMetric === name
                        ? 'bg-accent'
                        : 'hover:bg-accent/50'
                    }`}
                    onClick={() => handleSelectMetric(name)}
                  >
                    <code className="text-sm">{name}</code>
                    <Search className="h-4 w-4 text-muted-foreground" />
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
            {metricInfo ? (
              <div className="space-y-4">
                <div>
                  <h4 className="text-sm font-medium">Labels</h4>
                  <div className="flex flex-wrap gap-2 mt-2">
                    {metricInfo.label_names.map((label) => (
                      <Badge key={label} variant="outline">
                        {label}
                      </Badge>
                    ))}
                  </div>
                </div>
                <div>
                  <h4 className="text-sm font-medium">Latest Value</h4>
                  <p className="text-2xl font-bold mt-1">
                    {metricInfo.latest_value.toLocaleString()}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    at {new Date(metricInfo.latest_time).toLocaleString()}
                  </p>
                </div>
                <div>
                  <h4 className="text-sm font-medium">Total Data Points</h4>
                  <p className="text-lg">{metricInfo.total_points}</p>
                </div>
              </div>
            ) : (
              <p className="text-muted-foreground text-center py-8">
                Select a metric to view details
              </p>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Recent Samples */}
      <Card>
        <CardHeader>
          <CardTitle>Recent Metric Samples</CardTitle>
        </CardHeader>
        <CardContent>
          {metrics.length === 0 ? (
            <p className="text-muted-foreground text-center py-8">
              No recent samples
            </p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Metric</TableHead>
                  <TableHead>Value</TableHead>
                  <TableHead>Labels</TableHead>
                  <TableHead>Timestamp</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {metrics.slice(0, 20).map((metric, i) => (
                  <TableRow key={i}>
                    <TableCell className="font-mono">{metric.name}</TableCell>
                    <TableCell>{metric.value.toLocaleString()}</TableCell>
                    <TableCell>
                      <div className="flex flex-wrap gap-1">
                        {Object.entries(metric.labels).map(([k, v]) => (
                          <Badge key={k} variant="outline" className="text-xs">
                            {k}={v}
                          </Badge>
                        ))}
                      </div>
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {new Date(metric.timestamp).toLocaleString()}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
