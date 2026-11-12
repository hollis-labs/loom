import { Button, Pill } from "@hollis-labs/design-components"
import { FileDown } from "lucide-react"
import { useState } from "react"
import { type BundleExport, loom } from "../api/loom"
import { BundlePicker, ErrorState, messageOf, NoBundles, useBundles } from "./common"

export function ExportsPage() {
  const { bundles, slug, setSlug, error: bundlesError } = useBundles()
  const [result, setResult] = useState<BundleExport | null>(null)
  const [error, setError] = useState("")
  if (bundlesError) return <ErrorState error={bundlesError} />
  if (!bundles) return null
  if (bundles.length === 0) return <NoBundles />
  return (
    <div className="page-content stack form-width">
      <BundlePicker bundles={bundles} value={slug} onChange={setSlug} />
      <Button
        onClick={() =>
          void loom
            .exportBundle(slug)
            .then((r) => {
              setError("")
              setResult(r)
            })
            .catch((e: unknown) => setError(messageOf(e)))
        }
      >
        <FileDown className="h-4 w-4" /> Export bundle
      </Button>
      {error && <ErrorState error={error} />}
      {result && (
        <div className="panel stack">
          <div className="mono">{result.dir}</div>
          <div className="flex flex-wrap gap-2">
            {(result.files ?? []).map((file) => (
              <Pill key={file}>{file}</Pill>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
