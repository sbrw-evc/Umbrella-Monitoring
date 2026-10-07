import { SEVERITIES as ALL_SEVERITIES } from '../../incidents/types'
export type PDStatus = {
  enabled: boolean
  configured: boolean
  breaker_open: boolean
  consecutive_failures: number
  sent: number
  failed: number
  queue: number
  last_success_at?: string
  last_error?: string
  last_error_at?: string
  last_webhook_at?: string
}

export type PDRoute = {
  id: string
  name: string
  team_id?: string
  service_id?: string
  pd_service_id?: string
  pd_service_name?: string
  has_key: boolean
}

export type PagerDutyView = {
  enabled: boolean
  region: 'us' | 'eu'
  events_url?: string
  api_url?: string
  service_id?: string
  service_name?: string
  webhook_subscription_id?: string
  min_severity?: string
  updated_at?: string
  updated_by?: string
  routes: PDRoute[]
  has_routing_key: boolean
  has_api_token: boolean
  has_webhook_secret: boolean
  public_url: string
  webhook_url: string
  status: PDStatus
}

export type PDService = { id: string; name: string; html_url: string; status: string; events_key: boolean }

export type NotifyView = {
  email: {
    enabled: boolean
    host: string
    port: number
    security: 'starttls' | 'tls' | 'none'
    skip_verify: boolean
    username: string
    from: string
  }
  telegram: { enabled: boolean; api_url?: string }
  extra_emails: string[]
  extra_telegram: string[]
  updated_at?: string
  updated_by?: string
  has_password: boolean
  has_token: boolean
  public_url: string
  links: boolean
  // null: automatic (2 min while PagerDuty is on, at once while it is off).
  delay_seconds: number | null
  min_severity: string
  pd_enabled: boolean
  auto_delay_seconds: number
  // Message templates replaced by the administrator, by name ("fallback.text"…); absent: built-in.
  templates?: Record<string, string>
  default_templates?: Record<string, string>
}

// A message of the sample incident rendered by /api/notifications/preview.
export type NotifyPreview = { name: string; subject: string; text: string; html: string; error?: string }

export const TEMPLATE_MESSAGES = ['fallback', 'followup', 'test'] as const
export const TEMPLATE_PARTS = ['subject', 'text', 'html'] as const

export type Ref = { id: string; name: string }
export type Refs = { teams: Ref[]; services: Ref[] }

// The alert scale from the mildest up, as the minimum-severity lists show it.
export const SEVERITIES = [...ALL_SEVERITIES].reverse()
