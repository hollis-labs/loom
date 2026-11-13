/** Raised for any non-2xx answer. `code` is the flat error code, when the body carried one. */
export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
    public code = "",
  ) {
    super(message)
  }
}

/** Fired on a 401: the session ended. The identity gate returns to its sign-in screen. */
export const UNAUTHORIZED_EVENT = "loom:unauthorized"

/**
 * JSON over the same-origin cookie session. Errors are the flat `{code, message, details?}` body.
 * Every request carries X-Requested-With, which the backend requires on cookie writes. An empty
 * body (204) resolves to undefined. Sign-in, setup and the identity screens are the kit's own.
 */
export async function request<T>(path: string, method = "GET", body?: unknown): Promise<T> {
  const response = await fetch(`/api/${path}`, {
    method,
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", "X-Requested-With": "XMLHttpRequest" },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const text = await response.text()
  let data: { code?: string; message?: string } | null = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = null
    }
  }
  if (!response.ok) {
    if (response.status === 401) window.dispatchEvent(new Event(UNAUTHORIZED_EVENT))
    throw new ApiError(
      data?.message || "The request could not be completed.",
      response.status,
      data?.code ?? "",
    )
  }
  return data as T
}
