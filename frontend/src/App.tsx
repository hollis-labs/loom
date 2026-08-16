import { useState } from 'react'
import { Activity, Braces, Briefcase, FileDown, Files, ScrollText } from 'lucide-react'
import { NavRail, PageHeader, ThemeSwitcher, type NavRailItem } from '@hollis-labs/sysop-ui'
import { DashboardPage } from './pages/dashboard'

/**
 * App shell — the icon nav rail on the left, a pinned page header, and the
 * active page. Add pages by extending `nav` and the `route` switch below.
 */
export function App() {
  const [route, setRoute] = useState('bundles')

  const nav: NavRailItem[] = [
    { key: 'bundles', label: 'Bundles', icon: <Briefcase className="h-4 w-4" />, active: route === 'bundles', onSelect: () => setRoute('bundles') },
    { key: 'pages', label: 'Pages', icon: <Files className="h-4 w-4" />, active: route === 'pages', onSelect: () => setRoute('pages') },
    { key: 'jobs', label: 'Compile Jobs', icon: <ScrollText className="h-4 w-4" />, active: route === 'jobs', onSelect: () => setRoute('jobs') },
    { key: 'exports', label: 'Exports', icon: <FileDown className="h-4 w-4" />, active: route === 'exports', onSelect: () => setRoute('exports') },
    { key: 'directives', label: 'Directives', icon: <Braces className="h-4 w-4" />, active: route === 'directives', onSelect: () => setRoute('directives') },
  ]

  return (
    <div className="flex h-screen bg-bg text-text">
      <NavRail
        items={nav}
        logo={<Activity className="h-4 w-4" />}
        logoLabel="Loom"
        footerExtra={<ThemeSwitcher />}
      />
      <div className="flex min-w-0 flex-1 flex-col">
        <PageHeader title={nav.find((item) => item.key === route)?.label ?? 'Loom'} />
        <main className="min-h-0 flex-1 overflow-auto">
          <DashboardPage view={route as 'bundles' | 'pages' | 'jobs' | 'exports' | 'directives'} />
        </main>
      </div>
    </div>
  )
}
