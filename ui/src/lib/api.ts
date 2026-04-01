const API_BASE = '/api/v1'

export interface Plugin {
  id: string
  name: string
  version: string
  description?: string
  author?: string
  git_url: string
  git_commit?: string
  git_branch?: string
  ebpf_s3_key?: string
  wasm_s3_key?: string
  manifest: Record<string, unknown>
  status: 'pending' | 'building' | 'ready' | 'error'
  build_log?: string
  error_message?: string
  created_at: string
  updated_at: string
}

export interface Metric {
  id: string
  plugin_id: string
  name: string
  type: 'counter' | 'gauge' | 'histogram' | 'summary'
  help?: string
  labels: string[]
  created_at: string
}

export interface MetricSample {
  name: string
  value: number
  labels: Record<string, string>
  timestamp: string
  plugin_id?: string
}

export interface MetricInfo {
  name: string
  label_names: string[]
  label_values: Record<string, string[]>
  latest_value: number
  latest_time: string
  total_points: number
}

export interface Filter {
  id: string
  plugin_id?: string
  name: string
  expression: string
  description?: string
  is_default: boolean
  created_at: string
}

export interface Dashboard {
  id: string
  name: string
  description?: string
  config: {
    version: number
    panels: DashboardPanel[]
    layout: string
  }
  is_default: boolean
  created_at: string
  updated_at: string
}

export interface DashboardPanel {
  id: string
  title: string
  description?: string
  type: 'graph' | 'stat' | 'table' | 'heatmap'
  queries: DashboardQuery[]
  options?: Record<string, unknown>
  gridPos: {
    x: number
    y: number
    w: number
    h: number
  }
}

export interface DashboardQuery {
  id: string
  expression: string
  legend?: string
  format: 'time_series' | 'table'
}

export interface FilterResult {
  series: MetricSeries[]
}

export interface MetricSeries {
  name: string
  labels: Record<string, string>
  points: MetricPoint[]
}

export interface MetricPoint {
  value: number
  timestamp: string
}

// API Client
export const api = {
  // Plugins
  async getPlugins(): Promise<Plugin[]> {
    const res = await fetch(`${API_BASE}/plugins`)
    const data = await res.json()
    return data.data || []
  },

  async addPlugin(gitUrl: string, ref?: string): Promise<Plugin> {
    const res = await fetch(`${API_BASE}/plugins`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ git_url: gitUrl, ref }),
    })
    if (!res.ok) throw new Error(await res.text())
    return res.json()
  },

  async getPlugin(id: string): Promise<Plugin> {
    const res = await fetch(`${API_BASE}/plugins/${id}`)
    const data = await res.json()
    return data.data
  },

  async deletePlugin(id: string): Promise<void> {
    await fetch(`${API_BASE}/plugins/${id}`, { method: 'DELETE' })
  },

  async enablePlugin(id: string): Promise<void> {
    await fetch(`${API_BASE}/plugins/${id}/enable`, { method: 'POST' })
  },

  async disablePlugin(id: string): Promise<void> {
    await fetch(`${API_BASE}/plugins/${id}/disable`, { method: 'POST' })
  },

  async rebuildPlugin(id: string): Promise<void> {
    await fetch(`${API_BASE}/plugins/${id}/rebuild`, { method: 'POST' })
  },

  // Metrics
  async getMetrics(nameFilter?: string, labelFilter?: string): Promise<MetricSample[]> {
    const params = new URLSearchParams()
    if (nameFilter) params.set('name', nameFilter)
    if (labelFilter) params.set('label', labelFilter)
    const res = await fetch(`${API_BASE}/metrics?${params}`)
    const data = await res.json()
    return data.data || []
  },

  async getMetricByName(name: string): Promise<MetricInfo | null> {
    const res = await fetch(`${API_BASE}/metrics/${name}`)
    const data = await res.json()
    return data.data
  },

  async queryMetrics(query: string): Promise<MetricSeries[]> {
    const res = await fetch(`${API_BASE}/metrics/query`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ query }),
    })
    const data = await res.json()
    return data.data || []
  },

  async getMetricNames(): Promise<string[]> {
    const res = await fetch(`${API_BASE}/metrics/names`)
    const data = await res.json()
    return data.data || []
  },

  async getLabelValues(metric: string, label: string): Promise<string[]> {
    const res = await fetch(`${API_BASE}/metrics/${metric}/labels/${label}`)
    const data = await res.json()
    return data.data || []
  },

  // Filters
  async getFilters(pluginId?: string): Promise<Filter[]> {
    const params = pluginId ? `?plugin_id=${pluginId}` : ''
    const res = await fetch(`${API_BASE}/filters${params}`)
    const data = await res.json()
    return data.data || []
  },

  async createFilter(filter: Omit<Filter, 'id' | 'created_at'>): Promise<Filter> {
    const res = await fetch(`${API_BASE}/filters`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(filter),
    })
    if (!res.ok) throw new Error(await res.text())
    const data = await res.json()
    return data.data
  },

  async deleteFilter(id: string): Promise<void> {
    await fetch(`${API_BASE}/filters/${id}`, { method: 'DELETE' })
  },

  async executeFilter(expression: string): Promise<FilterResult> {
    const res = await fetch(`${API_BASE}/filters/execute`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ expression }),
    })
    const data = await res.json()
    return data.data
  },

  // Dashboard
  async getDashboard(id?: string): Promise<Dashboard | null> {
    const params = id ? `?id=${id}` : ''
    const res = await fetch(`${API_BASE}/dashboard${params}`)
    const data = await res.json()
    return data.data
  },

  async listDashboards(): Promise<Dashboard[]> {
    const res = await fetch(`${API_BASE}/dashboard/list`)
    const data = await res.json()
    return data.data || []
  },

  async saveDashboard(dashboard: Dashboard): Promise<Dashboard> {
    const res = await fetch(`${API_BASE}/dashboard`, {
      method: dashboard.id ? 'PUT' : 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(dashboard),
    })
    const data = await res.json()
    return data.data
  },

  async deleteDashboard(id: string): Promise<void> {
    await fetch(`${API_BASE}/dashboard/${id}`, { method: 'DELETE' })
  },
}

// WebSocket client
export class WebSocketClient {
  private ws: WebSocket | null = null
  private reconnectDelay = 1000
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null
  private messageHandlers: Set<(data: unknown) => void> = new Set()

  connect() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    this.ws = new WebSocket(`${protocol}//${window.location.host}/ws`)

    this.ws.onopen = () => {
      console.log('WebSocket connected')
      this.reconnectDelay = 1000
    }

    this.ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data)
        this.messageHandlers.forEach(handler => handler(data))
      } catch (err) {
        console.error('Failed to parse WebSocket message:', err)
      }
    }

    this.ws.onclose = () => {
      console.log('WebSocket disconnected, reconnecting...')
      this.reconnectTimer = setTimeout(() => this.connect(), this.reconnectDelay)
      this.reconnectDelay = Math.min(this.reconnectDelay * 2, 30000)
    }

    this.ws.onerror = (error) => {
      console.error('WebSocket error:', error)
    }
  }

  disconnect() {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer)
      this.reconnectTimer = null
    }
    if (this.ws) {
      this.ws.close()
      this.ws = null
    }
  }

  onMessage(handler: (data: unknown) => void) {
    this.messageHandlers.add(handler)
    return () => this.messageHandlers.delete(handler)
  }

  send(data: unknown) {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(data))
    }
  }
}

export const wsClient = new WebSocketClient()
