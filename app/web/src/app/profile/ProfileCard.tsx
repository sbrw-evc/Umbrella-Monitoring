import type { FormEvent, ReactNode } from 'react'
import { Banner } from '../../ui'
import type { Action } from './useAction'

export function ProfileCard({
  title,
  action,
  footer,
  onSubmit,
  wide,
  children,
}: {
  title: string
  action?: Action
  footer?: ReactNode
  onSubmit?: () => void
  wide?: boolean
  children: ReactNode
}) {
  const body = (
    <>
      <header>
        <h2>{title}</h2>
      </header>
      {children}
      {action?.notice && <Banner kind="ok" title={action.notice} />}
      {action?.error && (
        <Banner kind="error" title={action.error.message}>
          {action.error.detail}
        </Banner>
      )}
      {footer && <div className="card-actions">{footer}</div>}
    </>
  )
  const className = `card status-card${wide ? ' span-all' : ''}`
  if (!onSubmit) {
    return (
      <section className={className} aria-label={title}>
        {body}
      </section>
    )
  }
  const submit = (e: FormEvent) => {
    e.preventDefault()
    onSubmit()
  }
  return (
    <form className={className} aria-label={title} onSubmit={submit} noValidate>
      {body}
    </form>
  )
}
