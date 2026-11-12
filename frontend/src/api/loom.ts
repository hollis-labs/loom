import { request } from "./request"

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

const seg = encodeURIComponent

export const loom = {
  listBundles: () => request<Bundle[]>("bundles"),
  listPages: (bundle: string, q = "") => request<Page[]>(`pages?bundle=${seg(bundle)}&q=${seg(q)}`),
  listJobs: () => request<CompileJob[]>("compile-jobs"),
  createJob: (bundle: string, input: string) =>
    request<CompileJob>("compile-jobs", "POST", { bundle, generator: "wiki_page", input }),
  exportBundle: (bundle: string) =>
    request<BundleExport>(`bundles/${seg(bundle)}/export`, "POST", {}),
  parseDirectives: (text: string, save = false) =>
    request<DirectiveParse>("directives/parse", "POST", { text, source: "ui", save }),
}
