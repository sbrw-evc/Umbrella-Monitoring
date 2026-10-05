import { useCallback, useState } from 'react'
import { ApiError } from '../../api'
import { errorText, useT, type Dict } from '../../i18n'
import { useSession } from '../session'
import { strings } from '../strings'

export type Action = {
  busy: boolean
  error: { message: string; detail?: string } | null
  notice: string
  run: (fn: () => Promise<string | void>) => Promise<void>
  reset: () => void
}

export function useAction(dict: Dict = strings): Action {
  const t = useT(dict)
  const { expire, refresh } = useSession()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<Action['error']>(null)
  const [notice, setNotice] = useState('')

  const reset = useCallback(() => {
    setError(null)
    setNotice('')
  }, [])

  const run = useCallback(
    async (fn: () => Promise<string | void>) => {
      setBusy(true)
      reset()
      try {
        const message = await fn()
        if (message) setNotice(message)
      } catch (e) {
        if (e instanceof ApiError && e.status === 401) expire()
        else if (e instanceof ApiError && e.code === 'password_expired') await refresh()
        else setError(errorText(t, e))
      } finally {
        setBusy(false)
      }
    },
    [t, expire, refresh, reset],
  )

  return { busy, error, notice, run, reset }
}
