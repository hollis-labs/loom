import { ManifestShell, type PageComponent } from "@hollis-labs/app-shell"
import { ApiKeysPage, IDENTITY_PAGES, IdentityGate } from "@hollis-labs/kit-identity"
import { IdentityProvider, useIdentity } from "@hollis-labs/kit-identity/client"
import {
  Braces,
  Briefcase,
  FileDown,
  Files,
  KeyRound,
  LogOut,
  Monitor,
  ScrollText,
  Shield,
} from "lucide-react"
import { useCallback, useEffect, useState } from "react"
import { UNAUTHORIZED_EVENT } from "./api/request"
import { manifest } from "./manifest"
import { BundlesPage } from "./pages/bundles"
import { DirectivesPage } from "./pages/directives"
import { ExportsPage } from "./pages/exports"
import { JobsPage } from "./pages/jobs"
import { PagesPage } from "./pages/pages"

// Kit icon names used by the contributions (cmd/loom/ui/app.json), resolved by the shell.
const icons = {
  briefcase: Briefcase,
  files: Files,
  "scroll-text": ScrollText,
  "file-down": FileDown,
  braces: Braces,
  shield: Shield,
  "key-round": KeyRound,
  monitor: Monitor,
  "log-out": LogOut,
}

// The server has no scope catalogue; these are the scopes Loom's routes check.
const KEY_SCOPES = ["loom.read", "loom.write", "loom.mcp"]

const pages: Record<string, PageComponent> = {
  bundles: () => <BundlesPage />,
  pages: () => <PagesPage />,
  jobs: () => <JobsPage />,
  exports: () => <ExportsPage />,
  directives: () => <DirectivesPage />,
  "identity-account": () => <IDENTITY_PAGES.account />,
  "identity-keys": () => <ApiKeysPage scopeSuggestions={KEY_SCOPES} />,
  "identity-sessions": () => <IDENTITY_PAGES.sessions />,
  "identity-sign-out": () => <IDENTITY_PAGES.signOut />,
}

// An explicit sign-out leaves the URL on the sign-out page; signing in again must not land there.
const SIGN_OUT_PATH = "/identity/sign-out"

function initialPath() {
  if (window.location.pathname !== SIGN_OUT_PATH) return window.location.pathname
  window.history.replaceState(null, "", "/")
  return "/"
}

function usePath() {
  const [path, setPath] = useState(initialPath)
  useEffect(() => {
    const onPop = () => setPath(window.location.pathname)
    window.addEventListener("popstate", onPop)
    return () => window.removeEventListener("popstate", onPop)
  }, [])
  const navigate = useCallback((to: string) => {
    if (to !== window.location.pathname) window.history.pushState(null, "", to)
    setPath(to)
  }, [])
  return [path, navigate] as const
}

/**
 * The kit moves to its "session expired" sign-in when one of its own requests is answered 401.
 * The app's own requests report the same fact through UNAUTHORIZED_EVENT; this asks the kit a
 * question only a live session answers, so its 401 puts the gate on the sign-in screen.
 */
function SessionLossBridge() {
  const { client } = useIdentity()
  useEffect(() => {
    const onLoss = () => void client.listSessions().catch(() => {})
    window.addEventListener(UNAUTHORIZED_EVENT, onLoss)
    return () => window.removeEventListener(UNAUTHORIZED_EVENT, onLoss)
  }, [client])
  return null
}

function Shell() {
  const [path, navigate] = usePath()
  return (
    <ManifestShell
      manifest={manifest}
      apiBase="/api"
      path={path}
      onNavigate={navigate}
      pages={pages}
      icons={icons}
    />
  )
}

export function App() {
  return (
    // The gate renders first-run, sign-in and the no-owner screen outside the shell and mounts
    // the app only once a session exists.
    <IdentityProvider>
      <IdentityGate appName="loom">
        <SessionLossBridge />
        <Shell />
      </IdentityGate>
    </IdentityProvider>
  )
}
