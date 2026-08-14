/**
 * Web-app-only strings (dashboard KPIs, filters, timeline, onboarding steps)
 * that are not part of the shared cross-platform dictionary. Same pattern as
 * @frontdesk/shared: EN is the source shape, FR is typed against it.
 */
export const appEn = {
  language: {
    label: "Language",
    en: "English",
    fr: "Français",
  },
  login: {
    title: "Sign in to FrontDesk AI",
    subtitle: "Paste the API key you received during onboarding.",
    noAccount: "New here?",
    startOnboarding: "Set up your receptionist",
  },
  dashboard: {
    title: "Dashboard",
    callsToday: "Calls today",
    resolutionRate: "Resolved without a human",
    resolutionHint: "Ended calls the AI completed on its own (30 days)",
    p50Latency: "Turn latency (p50)",
    latencyHint: "Perceived response time, {{count}} recent calls",
    latencyTarget: "target < 1.2 s",
    scheduledJobs: "Upcoming jobs",
    estValue: "est. {{amount}}",
    estValueHint: "Estimate at {{amount}} per booked job",
    recentCalls: "Recent calls",
    viewAll: "View all",
    noCallsToday: "No calls yet today.",
  },
  filters: {
    outcome: "Outcome",
    language: "Language",
    urgency: "Urgency",
    any: "Any",
    clear: "Clear filters",
    noMatch: "No calls match the current filters.",
  },
  callDetail: {
    timeline: "Timeline",
    metrics: "Turn metrics",
    turn: "Turn {{n}}",
    recording: "Recording",
    noRecording: "This call was not recorded.",
    flowPath: "Flow path",
    langSwitched: "Language switched to {{locale}}",
    langDetected: "Language detected: {{locale}}",
    callStarted: "Call started",
    callAnswered: "Answered by the AI",
    callEnded: "Call ended",
    consentGranted: "Recording consent granted",
    consentDenied: "Recording consent denied",
    nodeEntered: "Entered step “{{node}}”",
    degradation: "Degradation stage: {{stage}}",
    errors_one: "{{count}} error during the call",
    errors_other: "{{count}} errors during the call",
    piiNote: "Personal information was redacted before storage.",
  },
  messagesPage: {
    markHandled: "Mark as handled",
    markUnhandled: "Mark as new",
    handled: "Handled",
    new: "New",
    showHandled: "Show handled",
    localNote: "Handled status is stored on this device.",
    call: "Call",
  },
  settingsPage: {
    title: "Settings",
    saved: "Settings saved",
    alwaysAiWarning:
      "In Always AI mode the AI answers every call immediately — your team's phones will not ring first.",
    hoursHint: "Outside these hours the AI answers immediately, regardless of mode.",
    closed: "Closed",
    addRange: "Add hours",
    remove: "Remove",
    coverageTitle: "Service area",
    ownerTitle: "Owner mobile",
    ownerHint: "Emergencies transfer here; you get a text after every call.",
    languageTitle: "Default language",
    readOnlyNote:
      "Business hours, service area, owner mobile and default language are set at onboarding; the API currently exposes only the answering-mode update. Contact support to change the rest.",
    numbersTitle: "Phone numbers",
  },
  onboarding: {
    stepBusiness: "Business",
    stepImport: "Website & FAQ",
    stepNumber: "Phone number",
    stepTest: "Test call",
    stepOf: "Step {{current}} of {{total}}",
    skipImport: "Skip — I'll add details later",
    importing: "Reading your website…",
    importFailed: "We could not read that site. You can continue without it.",
    nothingExtracted: "Nothing extracted yet.",
    didHint: "Your FrontDesk number. Callers dial it; the AI answers per your mode.",
    testTitle: "Give it a try",
    testBody:
      "Call {{number}} from your cell phone. The AI answers in {{locale}}, qualifies the caller and books or takes a message. Everything shows up in the dashboard within seconds.",
    testNoNumber: "Provision a number in the previous step to run a test call.",
    keyWarning: "Store this key in a password manager — it cannot be shown again.",
  },
  audio: {
    play: "Play",
    pause: "Pause",
    notSupported: "Your browser cannot play this recording.",
  },
} as const;

type DeepStringRecord<T> = {
  [K in keyof T]: T[K] extends string ? string : DeepStringRecord<T[K]>;
};

/** Shape every app dictionary must satisfy (FR is checked against EN). */
export type AppDictionary = DeepStringRecord<typeof appEn>;
