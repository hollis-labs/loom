import { createApiClient } from '@hollis-labs/sysop-ui/api'

// Same-origin: the Go binary serves both this SPA and the API, so an empty
// baseUrl resolves every request against the current origin.
const http = createApiClient({ baseUrl: '' })

export interface HealthInfo {
  status: string
}

export interface Bundle {
  id: number
  slug: string
  title: string
  description: string
  updated_at: string
}

export interface Page {
  id: number
  bundle_id: number
  slug: string
  title: string
  summary: string
  body: string
  updated_at: string
}

export interface CompileJob {
  id: number
  bundle_id: number
  generator: string
  status: string
  input: string
  output: string
  error: string
  updated_at: string
}

export interface DirectiveParse {
  directives: Array<{
    command: string
    prompt: string
    hash: string
    line: number
    category: string
    new: boolean
  }>
  warnings: Array<{ line: number; message: string }>
}

export interface BundleExport {
  bundle: Bundle
  dir: string
  files: string[]
}

export const apiClient = {
  getHealth: () => http.get<HealthInfo>('/api/health'),
  listBundles: () => http.get<Bundle[]>('/api/bundles'),
  listPages: (bundle = 'nanite', q = '') =>
    http.get<Page[]>(`/api/pages?bundle=${encodeURIComponent(bundle)}&q=${encodeURIComponent(q)}`),
  getPage: (bundle: string, slug: string) =>
    http.get<Page>(`/api/pages/${encodeURIComponent(bundle)}/${encodeURIComponent(slug)}`),
  listJobs: () => http.get<CompileJob[]>('/api/compile-jobs'),
  createJob: (bundle: string, input: string) =>
    http.post<CompileJob>('/api/compile-jobs', { bundle, generator: 'wiki_page', input }),
  exportBundle: (bundle: string) =>
    http.post<BundleExport>(`/api/bundles/${encodeURIComponent(bundle)}/export`, {}),
  parseDirectives: (text: string, save = false) =>
    http.post<DirectiveParse>('/api/directives/parse', { text, source: 'ui', save }),
}

export type AppApiClient = typeof apiClient
