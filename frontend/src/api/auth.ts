import { ApiError, createApiClient } from '@hollis-labs/sysop-ui/api'

const tokenKey = 'loom.apiToken'
const client = createApiClient({ baseUrl: '' })
let pendingPrompt: Promise<string | null> | undefined

function askForToken(): Promise<string | null> {
  // Share one prompt across requests that fail together during page loading.
  pendingPrompt ??= Promise.resolve().then(() => {
    const token = window.prompt('Enter the Loom API token:')?.trim() || null
    if (token) window.sessionStorage.setItem(tokenKey, token)
    return token
  }).finally(() => { pendingPrompt = undefined })
  return pendingPrompt
}

// Only same-origin API/MCP requests carry the token. It stays in this tab's
// sessionStorage and never enters a URL, log, or persistent localStorage.
export async function authenticatedRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const url = new URL(path, window.location.href)
  const protectedPath = /^\/(api(?:\/|$)|mcp(?:\/|$))/.test(url.pathname)
  if (url.origin !== window.location.origin || !protectedPath) {
    throw new Error('Loom API requests must use same-origin /api or /mcp paths')
  }
  const send = (token: string | null) => {
    const headers = Object.fromEntries(new Headers(init?.headers).entries())
    if (token) headers.Authorization = `Bearer ${token}`
    else delete headers.Authorization
    return client.request<T>(path, { ...init, headers })
  }
  const attemptedToken = window.sessionStorage.getItem(tokenKey)
  try {
    return await send(attemptedToken)
  } catch (error) {
    if (!(error instanceof ApiError) || error.status !== 401) throw error
    // A concurrent request may already have replaced the rejected token.
    let token = window.sessionStorage.getItem(tokenKey)
    if (!token || token === attemptedToken) {
      window.sessionStorage.removeItem(tokenKey)
      token = await askForToken()
    }
    if (!token) throw error
    try {
      return await send(token)
    } catch (retryError) {
      if (retryError instanceof ApiError && retryError.status === 401 &&
          window.sessionStorage.getItem(tokenKey) === token) {
        window.sessionStorage.removeItem(tokenKey)
      }
      throw retryError
    }
  }
}

export const authenticatedClient = {
  get: <T>(path: string) => authenticatedRequest<T>(path, { method: 'GET' }),
  post: <T>(path: string, body: Record<string, unknown>) => authenticatedRequest<T>(path, {
    method: 'POST', body: JSON.stringify(body), headers: { 'Content-Type': 'application/json' },
  }),
}
