import test from 'node:test'
import assert from 'node:assert/strict'
import { authenticatedRequest } from './auth.ts'

function browser({ token = null, answer = 'correct', accept = 'correct' } = {}) {
  const storage = new Map(token ? [['loom.apiToken', token]] : [])
  const calls = []
  let prompts = 0
  globalThis.window = {
    location: { href: 'http://localhost:8080/', origin: 'http://localhost:8080' },
    sessionStorage: {
      getItem: key => storage.get(key) ?? null,
      setItem: (key, value) => storage.set(key, value),
      removeItem: key => storage.delete(key),
    },
    prompt: () => { prompts++; return answer },
  }
  globalThis.fetch = async (url, init) => {
    const auth = new Headers(init.headers).get('Authorization')
    calls.push({ url, auth, body: init.body })
    const ok = accept === null || auth === `Bearer ${accept}`
    return new Response(JSON.stringify(ok ? { ok: true } : { error: 'unauthorized' }), {
      status: ok ? 200 : 401, headers: { 'Content-Type': 'application/json' },
    })
  }
  return { calls, storage, prompts: () => prompts }
}

test('loopback without a token never prompts', async () => {
  const state = browser({ accept: null })
  await authenticatedRequest('/api/bundles')
  assert.equal(state.prompts(), 0)
  assert.equal(state.calls[0].auth, null)
})

test('concurrent 401s share one prompt and retry API and MCP with the bearer', async () => {
  const state = browser({ token: 'expired' })
  await Promise.all([authenticatedRequest('/api/bundles'), authenticatedRequest('/mcp', { method: 'POST', body: '{}' })])
  assert.equal(state.prompts(), 1)
  assert.equal(state.storage.get('loom.apiToken'), 'correct')
  assert.equal(state.calls.filter(call => call.auth === 'Bearer correct').length, 2)
  assert.equal(state.calls.at(-1).body, '{}')
  assert.ok(state.calls.every(call => !call.url.includes('correct')))
})

test('a later 401 discards the stored bearer and prompts again', async () => {
  const state = browser({ token: 'expired', answer: 'replacement', accept: 'replacement' })
  await authenticatedRequest('/api/bundles')
  assert.equal(state.prompts(), 1)
  assert.equal(state.storage.get('loom.apiToken'), 'replacement')
})

test('canceling the prompt rejects without a retry', async () => {
  const state = browser({ answer: null })
  await assert.rejects(authenticatedRequest('/api/bundles'), error => error.status === 401)
  assert.equal(state.prompts(), 1)
  assert.equal(state.calls.length, 1)
  assert.equal(state.storage.size, 0)
})

test('a rejected token is cleared and retries are bounded', async () => {
  const state = browser({ answer: 'wrong' })
  await assert.rejects(authenticatedRequest('/api/bundles'), error => error.status === 401)
  assert.equal(state.calls.length, 2)
  assert.equal(state.storage.size, 0)
})

test('external and static paths cannot receive a token', async () => {
  const state = browser({ token: 'correct' })
  for (const path of ['https://example.com/api/bundles', '/assets/app.js']) {
    await assert.rejects(authenticatedRequest(path), /same-origin/)
  }
  assert.equal(state.calls.length, 0)
})
