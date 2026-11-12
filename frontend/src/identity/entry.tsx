import { Button, Input } from "@hollis-labs/design-components"
import { type FormEvent, useState } from "react"
import { ApiError, request } from "../api/request"
import { Field, Notice } from "./forms"

// Autofill hints for the password field.
const autofillNew = "new-password"
const autofillCurrent = "current-password"

/** First-run setup (when no owner exists yet) or sign-in. Both start a session cookie. */
export function Entry({ setup, onReady }: { setup: boolean; onReady: (name: string) => void }) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [mode, setMode] = useState(setup)
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = event.currentTarget
    const data = new FormData(form)
    if (mode && data.get("password") !== data.get("confirmation")) {
      setError("Passwords do not match.")
      return
    }
    setBusy(true)
    setError("")
    try {
      const profile = await request<{ display_name?: string; username?: string }>(
        mode ? "setup" : "login",
        "POST",
        {
          username: data.get("username"),
          password: data.get("password"),
          ...(mode ? { display_name: data.get("displayName") } : {}),
        },
      )
      form.reset()
      onReady(profile?.display_name || profile?.username || "")
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) setMode(false)
      setError(e instanceof Error ? e.message : "Could not sign in.")
    } finally {
      setBusy(false)
    }
  }
  return (
    <main className="entry">
      <section className="entry-panel">
        <h1>{mode ? "Create the owner account" : "Sign in to Loom"}</h1>
        <p className="text-fg-muted">
          {mode ? "Set up the owner account for this Loom." : "Pick up where you left off."}
        </p>
        <form onSubmit={submit} className="stack" aria-busy={busy}>
          <fieldset disabled={busy} className="stack">
            {mode && (
              <Field label="Display name">
                {(id) => (
                  <Input id={id} name="displayName" autoComplete="name" required autoFocus />
                )}
              </Field>
            )}
            <Field label="Username">
              {(id) => <Input id={id} name="username" autoComplete="username" required />}
            </Field>
            <Field label="Password">
              {(id) => (
                <Input
                  id={id}
                  name="password"
                  type="password"
                  autoComplete={mode ? autofillNew : autofillCurrent}
                  required
                />
              )}
            </Field>
            {mode && (
              <Field label="Confirm password">
                {(id) => (
                  <Input
                    id={id}
                    name="confirmation"
                    type="password"
                    autoComplete="new-password"
                    required
                  />
                )}
              </Field>
            )}
            <Notice error={error} />
            <Button type="submit" disabled={busy}>
              {busy ? "Please wait…" : mode ? "Create account" : "Sign in"}
            </Button>
          </fieldset>
        </form>
      </section>
    </main>
  )
}
