import { useEffect, useMemo, useState } from 'react'
import { FileDown, Play, RefreshCw, Search } from 'lucide-react'
import { EmptyState, SummaryCards } from '@hollis-labs/sysop-ui'
import { useApi } from '../api/context'
import type { Bundle, BundleExport, CompileJob, DirectiveParse, Page } from '../api/client'

type View = 'bundles' | 'pages' | 'jobs' | 'exports' | 'directives'

export function DashboardPage({ view }: { view: View }) {
  switch (view) {
    case 'pages':
      return <PagesView />
    case 'jobs':
      return <JobsView />
    case 'exports':
      return <ExportsView />
    case 'directives':
      return <DirectivesView />
    default:
      return <BundlesView />
  }
}

function BundlesView() {
  const api = useApi()
  const [bundles, setBundles] = useState<Bundle[]>([])
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    api.listBundles().then(setBundles).catch((err: unknown) => setError(message(err)))
  }, [api])

  if (error) return <ErrorState error={error} />

  return (
    <section className="space-y-4 p-6">
      <SummaryCards cards={[{ label: 'Bundles', value: String(bundles.length) }, { label: 'Canonical Store', value: 'SQLite' }]} />
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {bundles.map((bundle) => (
          <article key={bundle.id} className="rounded-md border border-border bg-panel p-4">
            <div className="text-sm font-semibold text-text">{bundle.title}</div>
            <div className="mt-1 font-mono text-xs text-text-muted">{bundle.slug}</div>
            <p className="mt-3 text-sm text-text-muted">{bundle.description}</p>
          </article>
        ))}
      </div>
    </section>
  )
}

function PagesView() {
  const api = useApi()
  const [query, setQuery] = useState('')
  const [pages, setPages] = useState<Page[]>([])
  const [selected, setSelected] = useState<Page | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    api.listPages('nanite', query).then((items) => {
      setPages(items)
      setSelected((current) => current ?? items[0] ?? null)
    }).catch((err: unknown) => setError(message(err)))
  }, [api, query])

  if (error) return <ErrorState error={error} />

  return (
    <section className="grid h-full min-h-0 grid-cols-[320px_1fr]">
      <aside className="border-r border-border p-4">
        <label className="flex items-center gap-2 rounded-md border border-border bg-bg px-3 py-2 text-sm">
          <Search className="h-4 w-4 text-text-muted" />
          <input className="min-w-0 flex-1 bg-transparent outline-none" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search pages" />
        </label>
        <div className="mt-4 space-y-2">
          {pages.map((page) => (
            <button key={page.id} type="button" onClick={() => setSelected(page)} className="block w-full rounded-md border border-border bg-panel p-3 text-left hover:bg-panel-hover">
              <div className="text-sm font-medium">{page.title}</div>
              <div className="mt-1 truncate text-xs text-text-muted">{page.summary || page.slug}</div>
            </button>
          ))}
        </div>
      </aside>
      <article className="min-w-0 overflow-auto p-6">
        {selected ? (
          <div className="max-w-3xl">
            <h2 className="text-xl font-semibold">{selected.title}</h2>
            <div className="mt-1 font-mono text-xs text-text-muted">{selected.slug}</div>
            <pre className="mt-5 whitespace-pre-wrap rounded-md border border-border bg-panel p-4 text-sm leading-6">{selected.body}</pre>
          </div>
        ) : (
          <EmptyState variant="empty" title="No pages yet" description="Compile a wiki page to populate the bundle." />
        )}
      </article>
    </section>
  )
}

function JobsView() {
  const api = useApi()
  const [jobs, setJobs] = useState<CompileJob[]>([])
  const [input, setInput] = useState('# New Wiki Page\n\nAdd source material here.')
  const [error, setError] = useState<string | null>(null)
  const refresh = () => api.listJobs().then(setJobs).catch((err: unknown) => setError(message(err)))

  useEffect(() => {
    refresh()
  }, [])

  const submit = () => api.createJob('nanite', input).then(refresh).catch((err: unknown) => setError(message(err)))
  if (error) return <ErrorState error={error} />

  return (
    <section className="grid gap-6 p-6 lg:grid-cols-[420px_1fr]">
      <div className="space-y-3">
        <textarea className="h-64 w-full resize-none rounded-md border border-border bg-panel p-3 font-mono text-sm outline-none" value={input} onChange={(event) => setInput(event.target.value)} />
        <button type="button" className="inline-flex items-center gap-2 rounded-md bg-accent px-3 py-2 text-sm font-medium text-accent-text" onClick={submit}>
          <Play className="h-4 w-4" /> Compile
        </button>
      </div>
      <JobTable jobs={jobs} />
    </section>
  )
}

function ExportsView() {
  const api = useApi()
  const [result, setResult] = useState<BundleExport | null>(null)
  const [error, setError] = useState<string | null>(null)
  const files = useMemo(() => result?.files ?? [], [result])

  return (
    <section className="space-y-4 p-6">
      <button type="button" className="inline-flex items-center gap-2 rounded-md bg-accent px-3 py-2 text-sm font-medium text-accent-text" onClick={() => api.exportBundle('nanite').then(setResult).catch((err: unknown) => setError(message(err)))}>
        <FileDown className="h-4 w-4" /> Export Nanite
      </button>
      {error && <ErrorState error={error} />}
      {result && <div className="rounded-md border border-border bg-panel p-4"><div className="font-mono text-sm">{result.dir}</div><div className="mt-3 flex flex-wrap gap-2">{files.map((file) => <span key={file} className="rounded bg-bg px-2 py-1 font-mono text-xs">{file}</span>)}</div></div>}
    </section>
  )
}

function DirectivesView() {
  const api = useApi()
  const [text, setText] = useState('::context_start Loom\nDiscuss wiki compiler shape.\n::note Capture architecture\n::context_end')
  const [parsed, setParsed] = useState<DirectiveParse | null>(null)
  const [error, setError] = useState<string | null>(null)

  return (
    <section className="grid gap-6 p-6 lg:grid-cols-[420px_1fr]">
      <div className="space-y-3">
        <textarea className="h-64 w-full resize-none rounded-md border border-border bg-panel p-3 font-mono text-sm outline-none" value={text} onChange={(event) => setText(event.target.value)} />
        <button type="button" className="inline-flex items-center gap-2 rounded-md bg-accent px-3 py-2 text-sm font-medium text-accent-text" onClick={() => api.parseDirectives(text, true).then(setParsed).catch((err: unknown) => setError(message(err)))}>
          <RefreshCw className="h-4 w-4" /> Parse
        </button>
      </div>
      {error ? <ErrorState error={error} /> : <pre className="overflow-auto rounded-md border border-border bg-panel p-4 text-xs">{JSON.stringify(parsed, null, 2)}</pre>}
    </section>
  )
}

function JobTable({ jobs }: { jobs: CompileJob[] }) {
  return <div className="overflow-hidden rounded-md border border-border bg-panel"><table className="w-full text-sm"><thead className="bg-bg text-left text-xs uppercase text-text-muted"><tr><th className="p-3">ID</th><th className="p-3">Generator</th><th className="p-3">Status</th></tr></thead><tbody>{jobs.map((job) => <tr key={job.id} className="border-t border-border"><td className="p-3 font-mono">{job.id}</td><td className="p-3">{job.generator}</td><td className="p-3">{job.status}</td></tr>)}</tbody></table></div>
}

function ErrorState({ error }: { error: string }) {
  return <EmptyState variant="error" title="Request failed" description={error} />
}

function message(err: unknown) {
  return err instanceof Error ? err.message : String(err)
}
