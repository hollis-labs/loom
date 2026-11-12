import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Metric,
} from "@hollis-labs/design-components"
import { ErrorState, NoBundles, useBundles } from "./common"

export function BundlesPage() {
  const { bundles, error } = useBundles()
  if (error) return <ErrorState error={error} />
  if (!bundles) return null
  return (
    <div className="page-content stack">
      <Metric label="Bundles" value={String(bundles.length)} />
      {bundles.length === 0 && <NoBundles />}
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {bundles.map((bundle) => (
          <Card key={bundle.id}>
            <CardHeader>
              <CardTitle>{bundle.title}</CardTitle>
              <CardDescription className="mono">{bundle.slug}</CardDescription>
            </CardHeader>
            <CardContent>
              <p className="text-fg-muted">{bundle.description}</p>
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  )
}
