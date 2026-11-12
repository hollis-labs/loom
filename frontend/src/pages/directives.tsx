import { Button, Textarea } from "@hollis-labs/design-components"
import { RefreshCw } from "lucide-react"
import { useState } from "react"
import { type DirectiveParse, loom } from "../api/loom"
import { ErrorState, messageOf } from "./common"

export function DirectivesPage() {
  const [text, setText] = useState(
    "::context_start Loom\nDiscuss wiki compiler shape.\n::note Capture architecture\n::context_end",
  )
  const [parsed, setParsed] = useState<DirectiveParse | null>(null)
  const [error, setError] = useState("")
  return (
    <div className="page-content split">
      <div className="stack">
        <Textarea
          aria-label="Directive text"
          className="h-64 font-mono"
          value={text}
          onChange={(e) => setText(e.target.value)}
        />
        <Button
          onClick={() =>
            void loom
              .parseDirectives(text, true)
              .then((p) => {
                setError("")
                setParsed(p)
              })
              .catch((e: unknown) => setError(messageOf(e)))
          }
        >
          <RefreshCw className="h-4 w-4" /> Parse
        </Button>
      </div>
      {error ? (
        <ErrorState error={error} />
      ) : (
        <pre className="panel body-text mono">{JSON.stringify(parsed, null, 2)}</pre>
      )}
    </div>
  )
}
