import { Ionicons } from "@expo/vector-icons";
import { Redirect, Tabs } from "expo-router";
import { useEffect, type ComponentProps, type ReactElement } from "react";
import { useTranslation } from "react-i18next";

import { LoadingView } from "../../src/components/ui";
import { apiBaseUrl } from "../../src/env";
import { registerForPushAsync } from "../../src/notifications";
import { useSession } from "../../src/session";
import { useTheme } from "../../src/theme";

type IconName = ComponentProps<typeof Ionicons>["name"];

function tabIcon(name: IconName) {
  return function Icon({ color, size }: { color: string; size: number }): ReactElement {
    return <Ionicons name={name} color={color} size={size} />;
  };
}

/** Auth gate + the four owner tabs: Home, Calls, Messages, Settings. */
export default function TabsLayout(): ReactElement {
  const { status, apiKey } = useSession();
  const theme = useTheme();
  const { t } = useTranslation();

  // Once signed in, register this device for post-call push summaries.
  useEffect(() => {
    if (status === "signedIn" && apiKey !== undefined) {
      void registerForPushAsync(apiBaseUrl, apiKey);
    }
  }, [status, apiKey]);

  if (status === "loading") {
    return <LoadingView />;
  }
  if (status === "signedOut") {
    return <Redirect href="/sign-in" />;
  }

  return (
    <Tabs
      screenOptions={{
        headerStyle: { backgroundColor: theme.colors.card },
        headerTitleStyle: { color: theme.colors.text },
        headerShadowVisible: false,
        tabBarStyle: {
          backgroundColor: theme.colors.card,
          borderTopColor: theme.colors.border,
        },
        tabBarActiveTintColor: theme.colors.accent,
        tabBarInactiveTintColor: theme.colors.textMuted,
        sceneStyle: { backgroundColor: theme.colors.background },
      }}
    >
      <Tabs.Screen
        name="index"
        options={{ title: t("mobile:tabs.home"), tabBarIcon: tabIcon("home") }}
      />
      <Tabs.Screen
        name="calls"
        options={{ title: t("mobile:tabs.calls"), tabBarIcon: tabIcon("call") }}
      />
      <Tabs.Screen
        name="messages"
        options={{
          title: t("mobile:tabs.messages"),
          tabBarIcon: tabIcon("chatbox-ellipses"),
        }}
      />
      <Tabs.Screen
        name="settings"
        options={{ title: t("mobile:tabs.settings"), tabBarIcon: tabIcon("settings") }}
      />
    </Tabs>
  );
}
