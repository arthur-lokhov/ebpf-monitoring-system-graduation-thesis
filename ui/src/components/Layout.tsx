import { Outlet, Link, useLocation } from 'react-router-dom'
import { Activity, BarChart3, LayoutDashboard, PlusCircle } from 'lucide-react'

export function Layout() {
  const location = useLocation()

  const navItems = [
    { path: '/plugins', label: 'Plugins', icon: PlusCircle },
    { path: '/metrics', label: 'Metrics', icon: Activity },
    { path: '/dashboard', label: 'Dashboard', icon: LayoutDashboard },
  ]

  return (
    <div className="min-h-screen bg-background">
      {/* Header */}
      <header className="sticky top-0 z-50 w-full border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
        <div className="container flex h-14 items-center">
          <div className="mr-4 flex">
            <Link to="/" className="mr-6 flex items-center space-x-2">
              <BarChart3 className="h-6 w-6" />
              <span className="font-bold">eBPF Monitoring</span>
            </Link>
            <nav className="flex items-center space-x-6 text-sm font-medium">
              {navItems.map((item) => {
                const Icon = item.icon
                const isActive = location.pathname.startsWith(item.path)
                return (
                  <Link
                    key={item.path}
                    to={item.path}
                    className={`flex items-center space-x-2 transition-colors hover:text-foreground/80 ${
                      isActive ? 'text-foreground' : 'text-foreground/60'
                    }`}
                  >
                    <Icon className="h-4 w-4" />
                    <span>{item.label}</span>
                  </Link>
                )
              })}
            </nav>
          </div>
        </div>
      </header>

      {/* Main content */}
      <main className="container py-6">
        <Outlet />
      </main>
    </div>
  )
}
