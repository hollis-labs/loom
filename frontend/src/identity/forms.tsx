import { type ReactNode, type Ref, useId } from "react"

export function Field({ label, children }: { label: string; children: (id: string) => ReactNode }) {
  const id = useId()
  return (
    <div className="field">
      <label htmlFor={id}>{label}</label>
      {children(id)}
    </div>
  )
}

/** An announced error. Pass `ref` to move focus to it (tabIndex -1: focusable, not in tab order). */
export function Notice({ error, ref }: { error?: string; ref?: Ref<HTMLParagraphElement> }) {
  return error ? (
    <p ref={ref} tabIndex={-1} className="text-danger" role="alert">
      {error}
    </p>
  ) : null
}
