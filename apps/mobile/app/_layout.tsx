import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Stack } from "expo-router";
import { StatusBar } from "expo-status-bar";
import { useEffect, useState, type ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { ActivityIndicator, View } from "react-native";

import { HandledMessagesProvider } from "../src/handled";
import { initI18n } from "../src/i18n";
import { useNotificationObserver } from "../src/notifications";
import { retryPolicy } from "../src/queries";
import { SessionProvider } from "../src/session";
import { useTheme } from "../src/theme";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 10_000,
      retry: retryPolicy,
    },
  },
});

export default function RootLayout(): ReactElement {
  const theme = useTheme();
  const [i18nReady, setI18nReady] = useState(false);

  useEffect(() => {
    void initI18n().then(() => {
      setI18nReady(true);
    });
  }, []);

  if (!i18nReady) {
    // Pre-i18n splash: plain spinner, no translated strings yet.
    return (
      <View
        style={{
          alignItems: "center",
          backgroundColor: theme.colors.background,
          flex: 1,
          justifyContent: "center",
        }}
      >
        <ActivityIndicator color={theme.colors.accent} />
      </View>
    );
  }

  return (
    <QueryClientProvider client={queryClient}>
      <SessionProvider>
        <HandledMessagesProvider>
          <StatusBar style={theme.dark ? "light" : "dark"} />
          <RootStack />
        </HandledMessagesProvider>
      </SessionProvider>
    </QueryClientProvider>
  );
}

function RootStack(): ReactElement {
  const theme = useTheme();
  const { t } = useTranslation();
  // Notification taps (post-call summaries) deep-link to /call/[id].
  useNotificationObserver();

  return (
    <Stack
      screenOptions={{
        headerStyle: { backgroundColor: theme.colors.card },
        headerTintColor: theme.colors.text,
        contentStyle: { backgroundColor: theme.colors.background },
      }}
    >
      <Stack.Screen name="(tabs)" options={{ headerShown: false }} />
      <Stack.Screen name="call/[id]" options={{ title: t("calls.detail") }} />
      <Stack.Screen name="sign-in" options={{ headerShown: false }} />
    </Stack>
  );
}
