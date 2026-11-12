import {
  Button,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  Textarea,
} from "@hollis-labs/design-components"
import { Play } from "lucide-react"
import { useCallback, useEffect, useState } from "react"
import { type CompileJob, loom } from "../api/loom"
import { BundlePicker, ErrorState, messageOf, NoBundles, useBundles } from "./common"

export function JobsPage() {
  const { bundles, slug, setSlug, error: bundlesError } = useBundles()
  const [jobs, setJobs] = useState<CompileJob[]>([])
  const [input, setInput] = useState("# New Wiki Page\n\nAdd source material here.")
  const [error, setError] = useState("")
  const refresh = useCallback(
    () =>
      loom
        .listJobs()
        .then((list) => setJobs(list ?? []))
        .catch((e: unknown) => setError(messageOf(e))),
    [],
  )
  useEffect(() => {
    void refresh()
  }, [refresh])
  const submit = () =>
    loom
      .createJob(slug, input)
      .then(refresh)
      .catch((e: unknown) => setError(messageOf(e)))
  if (bundlesError || error) return <ErrorState error={bundlesError || error} />
  if (!bundles) return null
  if (bundles.length === 0) return <NoBundles />
  return (
    <div className="page-content split">
      <div className="stack">
        <BundlePicker bundles={bundles} value={slug} onChange={setSlug} />
        <Textarea
          aria-label="Source material"
          className="h-64 font-mono"
          value={input}
          onChange={(e) => setInput(e.target.value)}
        />
        <Button onClick={() => void submit()}>
          <Play className="h-4 w-4" /> Compile
        </Button>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>ID</TableHead>
            <TableHead>Generator</TableHead>
            <TableHead>Status</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {jobs.map((job) => (
            <TableRow key={job.id}>
              <TableCell className="mono">{job.id}</TableCell>
              <TableCell>{job.generator}</TableCell>
              <TableCell>{job.status}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
