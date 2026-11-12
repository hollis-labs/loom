import { EmptyState } from "@hollis-labs/design-components"
import { useEffect, useState } from "react"
import { type Bundle, loom } from "../api/loom"

export function ErrorState({ error }: { error: string }) {
  return <EmptyState variant="error" title="Request failed" description={error} />
}

const LEGACY_BUNDLE = "nanite"

export const messageOf = (err: unknown) => (err instanceof Error ? err.message : String(err))

/** The bundles, and the one currently chosen (the first until the owner picks another). */
export function useBundles() {
  const [bundles, setBundles] = useState<Bundle[] | null>(null)
  const [slug, setSlug] = useState("")
  const [error, setError] = useState("")
  useEffect(() => {
    loom
      .listBundles()
      .then((list) => {
        const items = list ?? []
        setBundles(items)
        // The old UI always used this bundle; keep it as the first selection when it exists.
        const preferred = items.find((b) => b.slug === LEGACY_BUNDLE) ?? items[0]
        setSlug((current) => current || preferred?.slug || "")
      })
      .catch((e: unknown) => setError(messageOf(e)))
  }, [])
  return { bundles, slug, setSlug, error }
}

export function BundlePicker({
  bundles,
  value,
  onChange,
}: {
  bundles: Bundle[]
  value: string
  onChange: (slug: string) => void
}) {
  return (
    <label className="field">
      <span className="text-fg-muted">Bundle</span>
      <select
        className="rounded-md border border-border bg-bg px-3 py-2"
        value={value}
        onChange={(e) => onChange(e.target.value)}
      >
        {bundles.map((b) => (
          <option key={b.slug} value={b.slug}>
            {b.title || b.slug}
          </option>
        ))}
      </select>
    </label>
  )
}

export function NoBundles() {
  return (
    <EmptyState
      variant="empty"
      title="No bundles yet"
      description="Create a bundle with the loom CLI or the MCP tools, then it appears here."
    />
  )
}
