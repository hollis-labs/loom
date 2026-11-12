import { EmptyState, SearchInput } from "@hollis-labs/design-components"
import { useEffect, useState } from "react"
import { loom, type Page } from "../api/loom"
import { BundlePicker, ErrorState, messageOf, NoBundles, useBundles } from "./common"

export function PagesPage() {
  const { bundles, slug, setSlug, error: bundlesError } = useBundles()
  const [query, setQuery] = useState("")
  const [pages, setPages] = useState<Page[]>([])
  const [selected, setSelected] = useState<Page | null>(null)
  const [error, setError] = useState("")
  useEffect(() => {
    if (!slug) return
    // A slower answer for a superseded bundle or query must not overwrite the current one.
    let current = true
    loom
      .listPages(slug, query)
      .then((items) => {
        if (!current) return
        const list = items ?? []
        setError("")
        setPages(list)
        setSelected((prev) => list.find((p) => p.id === prev?.id) ?? list[0] ?? null)
      })
      .catch((e: unknown) => {
        if (current) setError(messageOf(e))
      })
    return () => {
      current = false
    }
  }, [slug, query])
  if (bundlesError || error) return <ErrorState error={bundlesError || error} />
  if (!bundles) return null
  if (bundles.length === 0) return <NoBundles />
  return (
    <div className="page-content split">
      <aside className="stack">
        <BundlePicker bundles={bundles} value={slug} onChange={setSlug} />
        <SearchInput
          value={query}
          onChange={setQuery}
          placeholder="Search pages"
          ariaLabel="Search pages"
        />
        {pages.map((page) => (
          <button
            key={page.id}
            type="button"
            onClick={() => setSelected(page)}
            className="panel text-left"
            aria-current={selected?.id === page.id}
          >
            <div className="font-medium">{page.title}</div>
            <div className="truncate text-fg-muted">{page.summary || page.slug}</div>
          </button>
        ))}
      </aside>
      <article className="min-w-0">
        {selected ? (
          <div className="stack">
            <h2 className="text-xl font-semibold">{selected.title}</h2>
            <div className="mono text-fg-muted">{selected.slug}</div>
            <pre className="panel body-text">{selected.body}</pre>
          </div>
        ) : (
          <EmptyState
            variant="empty"
            title="No pages yet"
            description="Compile a wiki page to populate the bundle."
          />
        )}
      </article>
    </div>
  )
}
