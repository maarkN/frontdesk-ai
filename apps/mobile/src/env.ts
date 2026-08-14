import Constants from "expo-constants";

interface Extra {
  apiBaseUrl?: string;
  eas?: { projectId?: string };
}

const extra: Extra = (Constants.expoConfig?.extra as Extra | undefined) ?? {};

/** Base URL of the Go core-api. Configured in app.json `expo.extra.apiBaseUrl`. */
export const apiBaseUrl: string = extra.apiBaseUrl ?? "http://localhost:8080";

/** EAS project id, required by getExpoPushTokenAsync outside Expo Go. */
export const easProjectId: string | undefined = extra.eas?.projectId;
