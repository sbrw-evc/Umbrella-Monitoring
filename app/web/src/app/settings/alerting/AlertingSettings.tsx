import { useState } from 'react'
import { NotifyCard } from './NotifyCard'
import { PagerDutyCard } from './PagerDutyCard'
import './alerting.css'

export function AlertingSettings() {
  // The notification card shows whether links work, which depends on the public address saved
  // with PagerDuty.
  const [epoch, setEpoch] = useState(0)
  return (
    <>
      <PagerDutyCard onSaved={() => setEpoch((e) => e + 1)} />
      <NotifyCard key={epoch} />
    </>
  )
}
