import { useState, useEffect, useCallback } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { PlusCircle, Trash2, RefreshCw, Play, Square, GitBranch } from 'lucide-react'
import { api, type Plugin } from '@/lib/api'

export function PluginsPage() {
  const [plugins, setPlugins] = useState<Plugin[]>([])
  const [loading, setLoading] = useState(true)
  const [gitUrl, setGitUrl] = useState('')
  const [gitRef, setGitRef] = useState('')
  const [adding, setAdding] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const loadPlugins = useCallback(async () => {
    try {
      const data = await api.getPlugins()
      setPlugins(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load plugins')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadPlugins()
  }, [loadPlugins])

  const handleAddPlugin = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!gitUrl.trim()) return

    setAdding(true)
    setError(null)
    try {
      await api.addPlugin(gitUrl, gitRef || undefined)
      setGitUrl('')
      setGitRef('')
      await loadPlugins()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to add plugin')
    } finally {
      setAdding(false)
    }
  }

  const handleDelete = async (id: string) => {
    if (!confirm('Are you sure you want to delete this plugin?')) return
    try {
      await api.deletePlugin(id)
      await loadPlugins()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete plugin')
    }
  }

  const handleEnable = async (id: string) => {
    try {
      await api.enablePlugin(id)
      await loadPlugins()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to enable plugin')
    }
  }

  const handleDisable = async (id: string) => {
    try {
      await api.disablePlugin(id)
      await loadPlugins()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to disable plugin')
    }
  }

  const handleRebuild = async (id: string) => {
    try {
      await api.rebuildPlugin(id)
      await loadPlugins()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to rebuild plugin')
    }
  }

  const getStatusBadge = (status: string) => {
    const variants: Record<string, 'default' | 'success' | 'warning' | 'destructive'> = {
      ready: 'success',
      building: 'default',
      pending: 'warning',
      error: 'destructive',
    }
    return <Badge variant={variants[status] || 'default'}>{status}</Badge>
  }

  if (loading) {
    return <div className="flex items-center justify-center h-64">Loading...</div>
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Plugins</h1>
        <p className="text-muted-foreground">
          Manage your eBPF monitoring plugins
        </p>
      </div>

      {error && (
        <Card className="border-destructive">
          <CardContent className="pt-6">
            <p className="text-destructive">{error}</p>
          </CardContent>
        </Card>
      )}

      {/* Add Plugin Form */}
      <Card>
        <CardHeader>
          <CardTitle>Add Plugin</CardTitle>
          <CardDescription>
            Add a new plugin from a Git repository
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleAddPlugin} className="flex gap-4">
            <Input
              placeholder="Git repository URL (e.g., https://github.com/user/plugin)"
              value={gitUrl}
              onChange={(e) => setGitUrl(e.target.value)}
              className="flex-1"
              disabled={adding}
            />
            <Input
              placeholder="Branch/Tag (optional)"
              value={gitRef}
              onChange={(e) => setGitRef(e.target.value)}
              className="w-48"
              disabled={adding}
            />
            <Button type="submit" disabled={adding || !gitUrl.trim()}>
              <PlusCircle className="h-4 w-4 mr-2" />
              Add
            </Button>
          </form>
        </CardContent>
      </Card>

      {/* Plugins List */}
      <Card>
        <CardHeader>
          <CardTitle>Installed Plugins</CardTitle>
          <CardDescription>
            {plugins.length} plugin{plugins.length !== 1 ? 's' : ''} installed
          </CardDescription>
        </CardHeader>
        <CardContent>
          {plugins.length === 0 ? (
            <p className="text-muted-foreground text-center py-8">
              No plugins installed. Add your first plugin above.
            </p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Version</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Repository</TableHead>
                  <TableHead>Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {plugins.map((plugin) => (
                  <TableRow key={plugin.id}>
                    <TableCell>
                      <div>
                        <div className="font-medium">{plugin.name}</div>
                        {plugin.description && (
                          <div className="text-sm text-muted-foreground">
                            {plugin.description}
                          </div>
                        )}
                      </div>
                    </TableCell>
                    <TableCell>{plugin.version}</TableCell>
                    <TableCell>{getStatusBadge(plugin.status)}</TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2 text-sm">
                        <GitBranch className="h-4 w-4" />
                        <a
                          href={plugin.git_url}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="hover:underline"
                        >
                          {plugin.git_url.replace(/^https?:\/\//, '')}
                        </a>
                        {plugin.git_branch && (
                          <Badge variant="secondary">{plugin.git_branch}</Badge>
                        )}
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="flex gap-2">
                        {plugin.status === 'ready' && (
                          <>
                            <Button
                              variant="outline"
                              size="icon"
                              onClick={() => handleDisable(plugin.id)}
                              title="Disable"
                            >
                              <Square className="h-4 w-4" />
                            </Button>
                          </>
                        )}
                        {plugin.status !== 'ready' && plugin.status !== 'building' && (
                          <Button
                            variant="outline"
                            size="icon"
                            onClick={() => handleEnable(plugin.id)}
                            title="Enable"
                          >
                            <Play className="h-4 w-4" />
                          </Button>
                        )}
                        <Button
                          variant="outline"
                          size="icon"
                          onClick={() => handleRebuild(plugin.id)}
                          title="Rebuild"
                        >
                          <RefreshCw className="h-4 w-4" />
                        </Button>
                        <Button
                          variant="outline"
                          size="icon"
                          onClick={() => handleDelete(plugin.id)}
                          title="Delete"
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
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
