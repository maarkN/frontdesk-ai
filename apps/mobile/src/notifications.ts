/**
 * Post-call push summaries (RF3): the notifier service sends a push after
 * every call; tapping it deep-links into the call detail.
 *
 * Token registration: core-api's /v1 surface (internal/api/openapi.yaml) does
 * not expose a push-token endpoint yet, so registration POSTs to the agreed
 * upcoming path `/v1/push-tokens` with the same Bearer auth the shared client
 * uses, and tolerates 404 until the endpoint ships. Once it lands in
 * @frontdesk/shared's ApiClient this file should switch to that method.
 */
import * as Device from "expo-device";
import * as Notifications from "expo-notifications";
import { router } from "expo-router";
import { useEffect } from "react";
import { Platform } from "react-native";

import { easProjectId } from "./env";

Notifications.setNotificationHandler({
  handleNotification: () =>
    Promise.resolve({
      shouldShowBanner: true,
      shouldShowList: true,
      shouldPlaySound: true,
      shouldSetBadge: false,
    }),
});

/** Returns true when permission is granted (asking on first run). */
export async function ensurePushPermission(): Promise<boolean> {
  if (!Device.isDevice) {
    return false; // Simulators cannot receive push.
  }
  const current = await Notifications.getPermissionsAsync();
  if (current.granted) {
    return true;
  }
  if (!current.canAskAgain) {
    return false;
  }
  const requested = await Notifications.requestPermissionsAsync();
  return requested.granted;
}

/**
 * Registers this device for post-call summaries: permission, Android channel,
 * Expo push token, then best-effort registration with core-api.
 */
export async function registerForPushAsync(
  baseUrl: string,
  apiKey: string,
): Promise<boolean> {
  const granted = await ensurePushPermission();
  if (!granted) {
    return false;
  }

  if (Platform.OS === "android") {
    await Notifications.setNotificationChannelAsync("post-call", {
      name: "Call summaries",
      importance: Notifications.AndroidImportance.HIGH,
      vibrationPattern: [0, 250, 250, 250],
    });
  }

  let token: string;
  try {
    const response =
      easProjectId === undefined
        ? await Notifications.getExpoPushTokenAsync()
        : await Notifications.getExpoPushTokenAsync({ projectId: easProjectId });
    token = response.data;
  } catch {
    return false; // No EAS project configured (e.g. bare dev) — skip quietly.
  }

  await sendTokenToCoreApi(baseUrl, apiKey, token);
  return true;
}

async function sendTokenToCoreApi(
  baseUrl: string,
  apiKey: string,
  token: string,
): Promise<void> {
  try {
    await fetch(`${baseUrl.replace(/\/+$/, "")}/v1/push-tokens`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${apiKey}`,
      },
      body: JSON.stringify({ token, platform: Platform.OS }),
    });
  } catch {
    // Best-effort: the app still works without push registration.
  }
}

function openFromResponse(response: Notifications.NotificationResponse | null): void {
  if (response === null) {
    return;
  }
  const data = response.notification.request.content.data as
    | Record<string, unknown>
    | undefined;
  const callId = data?.["callId"];
  if (typeof callId === "string" && callId !== "") {
    try {
      router.push({ pathname: "/call/[id]", params: { id: callId } });
    } catch {
      // Router not mounted yet (cold start race) — the summary is still
      // visible on the Home screen.
    }
  }
}

/**
 * Routes notification taps (foreground, background and cold start) to the
 * call detail screen. Mounted once in the root layout.
 */
export function useNotificationObserver(): void {
  useEffect(() => {
    let active = true;
    void Notifications.getLastNotificationResponseAsync().then((response) => {
      if (active) {
        openFromResponse(response);
      }
    });
    const subscription =
      Notifications.addNotificationResponseReceivedListener(openFromResponse);
    return () => {
      active = false;
      subscription.remove();
    };
  }, []);
}
