/**
 * Mobile-only UI strings, added as the "mobile" i18next namespace on top of
 * the shared EN/FR dictionaries (@frontdesk/shared). Same convention as the
 * shared dictionaries: EN is the source shape, FR is typed against it so a
 * missing key fails typecheck.
 */
export const mobileEn = {
  tabs: {
    home: "Home",
    calls: "Calls",
    messages: "Messages",
    settings: "Settings",
  },
  home: {
    greeting: "Hi, {{name}}",
    today: "Today",
    answered: "Calls answered",
    messagesTaken: "Messages",
    minutes: "Minutes",
    urgent: "Needs your attention",
    allClear: "All clear — nothing urgent.",
    upcoming: "Upcoming appointments",
    noUpcoming: "No upcoming appointments.",
    seeAll: "See all",
  },
  messageActions: {
    callBack: "Call back",
    markHandled: "Mark handled",
    handled: "Handled",
    handledSection: "Handled",
    openSection: "To handle",
  },
  settings: {
    hoursTitle: "Business hours",
    closed: "Closed",
    languageTitle: "App language",
    english: "English",
    french: "Français",
    notificationsTitle: "Notifications",
    pushEnabled: "Post-call summaries enabled",
    pushDisabled: "Push is off — allow notifications in system settings",
    enablePush: "Enable notifications",
    server: "API server",
    signOut: "Sign out",
  },
  signIn: {
    tagline: "Your AI receptionist, in your pocket.",
    hint: "Paste the API key you received during onboarding.",
  },
  days: {
    mon: "Monday",
    tue: "Tuesday",
    wed: "Wednesday",
    thu: "Thursday",
    fri: "Friday",
    sat: "Saturday",
    sun: "Sunday",
  },
} as const;

type DeepStringRecord<T> = {
  [K in keyof T]: T[K] extends string ? string : DeepStringRecord<T[K]>;
};

/** Shape every mobile dictionary must satisfy. */
export type MobileDictionary = DeepStringRecord<typeof mobileEn>;

export const mobileFr: MobileDictionary = {
  tabs: {
    home: "Accueil",
    calls: "Appels",
    messages: "Messages",
    settings: "Réglages",
  },
  home: {
    greeting: "Bonjour, {{name}}",
    today: "Aujourd'hui",
    answered: "Appels répondus",
    messagesTaken: "Messages",
    minutes: "Minutes",
    urgent: "Requiert votre attention",
    allClear: "Tout est calme — rien d'urgent.",
    upcoming: "Rendez-vous à venir",
    noUpcoming: "Aucun rendez-vous à venir.",
    seeAll: "Tout voir",
  },
  messageActions: {
    callBack: "Rappeler",
    markHandled: "Marquer traité",
    handled: "Traité",
    handledSection: "Traités",
    openSection: "À traiter",
  },
  settings: {
    hoursTitle: "Heures d'ouverture",
    closed: "Fermé",
    languageTitle: "Langue de l'application",
    english: "English",
    french: "Français",
    notificationsTitle: "Notifications",
    pushEnabled: "Résumés après appel activés",
    pushDisabled: "Notifications désactivées — autorisez-les dans les réglages du système",
    enablePush: "Activer les notifications",
    server: "Serveur API",
    signOut: "Se déconnecter",
  },
  signIn: {
    tagline: "Votre réceptionniste IA, dans votre poche.",
    hint: "Collez la clé API reçue lors de l'intégration.",
  },
  days: {
    mon: "Lundi",
    tue: "Mardi",
    wed: "Mercredi",
    thu: "Jeudi",
    fri: "Vendredi",
    sat: "Samedi",
    sun: "Dimanche",
  },
};
