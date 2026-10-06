export type StepID = 'source' | 'event' | 'team' | 'catalog' | 'delivery' | 'public_url'

export type OnboardingStep = { id: StepID; done: boolean; optional: boolean; path: string; can_fix: boolean }

export type Onboarding = { done: boolean; steps: OnboardingStep[] }
