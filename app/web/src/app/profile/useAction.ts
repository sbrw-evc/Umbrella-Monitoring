import { useCallback, useState } from 'react'
import { ApiError } from '../../api'
import { errorText, useT } from '../../i18n'
import { useSession } from '../session'
import { strings } from '../strings'

export type Action = {
  busy: boolean
  error: { message: string; detail?: string } | null
  notice: string
  run: (fn: () => Promise<string | void>) => Promise<void>
}

export function useAction(): Action {
  const t = useT(strings)
  const { expire } = useSession()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<Action['error']>(null)
  const [notice, setNotice] = useState('')

  const run = useCallback(
    async (fn: () => Promise<string | void>) => {
      setBusy(true)
      setError(null)
      setNotice('')
      try {
        const message = await fn()
        if (message) setNotice(message)
      } catch (e) {
        if (e instanceof ApiError && e.status === 401) expire()
        else setError(errorText(t, e))
      } finally {
        setBusy(false)
      }
    },
    [t, expire],
  )

  return { busy, error, notice, run }
}
