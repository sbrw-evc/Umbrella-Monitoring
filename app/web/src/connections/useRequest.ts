import { useCallback, useEffect, useRef, useState } from 'react'
import { api, ApiError } from '../api'
import { useSession } from '../app/session'

function useExpiry() {
  const { expire } = useSession()
  return useCallback(
    (e: unknown) => {
      if (e instanceof ApiError && e.status === 401) expire()
    },
    [expire],
  )
}

export function useResource<T>(path: string, epoch: number) {
  const onError = useExpiry()
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState<unknown>(null)
  const [busy, setBusy] = useState(false)
  const seq = useRef(0)
  // An empty path loads nothing, for resources only needed sometimes.
  const reload = useCallback(async () => {
    const mine = ++seq.current
    if (!path) return
    setBusy(true)
    try {
      const next = await api<T>('GET', path)
      if (mine !== seq.current) return
      setData(next)
      setError(null)
    } catch (e) {
      if (mine !== seq.current) return
      onError(e)
      setError(e)
    } finally {
      if (mine === seq.current) setBusy(false)
    }
  }, [path, onError])
  useEffect(() => {
    void reload()
  }, [reload, epoch])
  return { data, error, busy, reload }
}

export function useAction() {
  const onError = useExpiry()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const run = useCallback(
    async <T>(fn: () => Promise<T>): Promise<T | undefined> => {
      setBusy(true)
      setError(null)
      try {
        return await fn()
      } catch (e) {
        onError(e)
        setError(e)
        return undefined
      } finally {
        setBusy(false)
      }
    },
    [onError],
  )
  return { busy, error, run, clear: () => setError(null) }
}
