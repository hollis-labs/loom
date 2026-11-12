import { createContext, useContext } from "react"

/** What the UI knows about the signed-in caller. There is no profile read route yet. */
export interface Session {
  /** Display name, known only for a sign-in in this page's life (not after a reload). */
  name: string
  /** Drop back to the sign-in screen. */
  signedOut: () => void
}

export const SessionContext = createContext<Session | null>(null)

export function useSession(): Session {
  const s = useContext(SessionContext)
  if (!s) throw new Error("Session context missing")
  return s
}
