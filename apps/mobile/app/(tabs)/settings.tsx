/**
 * Settings — the mode toggle (the product's #1 control), business hours,
 * EN/FR app language, push status and sign-out.
 */
import * as Notifications from "expo-notifications";
import { useEffect, useState, type ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { Pressable, ScrollView, StyleSheet, Text, View } from "react-native";

import type { BusinessHours, DayKey, Locale, TimeRange } from "@frontdesk/shared";

import { ModeControl } from "../../src/components/ModeControl";
import { Card, ErrorView, LoadingView, PillButton, SectionHeader } from "../../src/components/ui";
import { apiBaseUrl } from "../../src/env";
import { currentUiLocale, setUiLocale } from "../../src/i18n";
import { registerForPushAsync } from "../../src/notifications";
import { useTenantQuery } from "../../src/queries";
import { useSession } from "../../src/session";
import { useTheme } from "../../src/theme";

const DAY_ORDER: readonly DayKey[] = ["mon", "tue", "wed", "thu", "fri", "sat", "sun"];

export default function SettingsScreen(): ReactElement {
  const theme = useTheme();
  const { t, i18n } = useTranslation();
  const { apiKey, signOut } = useSession();
  const tenantQuery = useTenantQuery();
  const [pushGranted, setPushGranted] = useState<boolean | undefined>(undefined);

  useEffect(() => {
    let cancelled = false;
    void Notifications.getPermissionsAsync().then((permissions) => {
      if (!cancelled) {
        setPushGranted(permissions.granted);
      }
    });
    return () => {
      cancelled = true;
    };
  }, []);

  if (tenantQuery.isPending) {
    return <LoadingView />;
  }
  if (tenantQuery.isError) {
    return (
      <ErrorView
        message={t("errors.network")}
        onRetry={() => {
          void tenantQuery.refetch();
        }}
      />
    );
  }
  const tenant = tenantQuery.data;
  const uiLocale = currentUiLocale();

  const chooseLocale = (locale: Locale): void => {
    if (locale !== uiLocale) {
      void setUiLocale(locale);
    }
  };

  const enablePush = (): void => {
    if (apiKey === undefined) {
      return;
    }
    void registerForPushAsync(apiBaseUrl, apiKey).then(setPushGranted);
  };

  const localeChoice = (locale: Locale, label: string): ReactElement => {
    const active = uiLocale === locale;
    return (
      <Pressable
        accessibilityRole="button"
        accessibilityState={{ selected: active }}
        onPress={() => {
          chooseLocale(locale);
        }}
        style={[
          styles.localeChip,
          {
            backgroundColor: active ? theme.colors.accentSoft : "transparent",
            borderColor: active ? theme.colors.accent : theme.colors.border,
          },
        ]}
      >
        <Text
          style={[
            styles.localeChipText,
            { color: active ? theme.colors.accent : theme.colors.textMuted },
          ]}
        >
          {label}
        </Text>
      </Pressable>
    );
  };

  return (
    <ScrollView
      style={{ backgroundColor: theme.colors.background }}
      contentContainerStyle={styles.content}
      // Re-render day rows when the language flips.
      key={i18n.language}
    >
      <ModeControl tenant={tenant} />

      <SectionHeader title={t("mobile:settings.hoursTitle")} />
      <Card>
        {DAY_ORDER.map((day) => (
          <HoursRow key={day} day={day} hours={tenant.hours} />
        ))}
      </Card>

      <SectionHeader title={t("mobile:settings.languageTitle")} />
      <Card>
        <View style={styles.localeRow}>
          {localeChoice("en-CA", t("mobile:settings.english"))}
          {localeChoice("fr-CA", t("mobile:settings.french"))}
        </View>
      </Card>

      <SectionHeader title={t("mobile:settings.notificationsTitle")} />
      <Card>
        <Text
          style={[
            styles.pushText,
            { color: pushGranted === true ? theme.colors.success : theme.colors.textMuted },
          ]}
        >
          {pushGranted === true
            ? t("mobile:settings.pushEnabled")
            : t("mobile:settings.pushDisabled")}
        </Text>
        {pushGranted === true ? null : (
          <View style={styles.pushButton}>
            <PillButton
              label={t("mobile:settings.enablePush")}
              tone="neutral"
              onPress={enablePush}
            />
          </View>
        )}
      </Card>

      <SectionHeader title={t("mobile:settings.server")} />
      <Card>
        <Text style={[styles.server, { color: theme.colors.textMuted }]}>{apiBaseUrl}</Text>
      </Card>

      <View style={styles.signOut}>
        <PillButton
          label={t("mobile:settings.signOut")}
          tone="danger"
          onPress={() => {
            void signOut();
          }}
        />
      </View>
    </ScrollView>
  );
}

function HoursRow({
  day,
  hours,
}: {
  day: DayKey;
  hours: BusinessHours | undefined;
}): ReactElement {
  const theme = useTheme();
  const { t } = useTranslation();
  const ranges: TimeRange[] = hours?.[day] ?? [];
  return (
    <View style={styles.hoursRow}>
      <Text style={[styles.hoursDay, { color: theme.colors.text }]}>
        {t(`mobile:days.${day}`)}
      </Text>
      <Text
        style={[
          styles.hoursValue,
          {
            color: ranges.length === 0 ? theme.colors.textMuted : theme.colors.text,
          },
        ]}
      >
        {ranges.length === 0
          ? t("mobile:settings.closed")
          : ranges.map((range) => `${range.open}–${range.close}`).join(", ")}
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  content: {
    padding: 16,
    paddingBottom: 32,
  },
  hoursRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    paddingVertical: 5,
  },
  hoursDay: {
    fontSize: 14,
    fontWeight: "600",
  },
  hoursValue: {
    fontSize: 14,
    fontVariant: ["tabular-nums"],
  },
  localeRow: {
    flexDirection: "row",
    gap: 8,
  },
  localeChip: {
    borderRadius: 999,
    borderWidth: 1,
    paddingHorizontal: 18,
    paddingVertical: 8,
  },
  localeChipText: {
    fontSize: 14,
    fontWeight: "600",
  },
  pushText: {
    fontSize: 14,
  },
  pushButton: {
    alignItems: "flex-start",
    marginTop: 10,
  },
  server: {
    fontSize: 13,
    fontVariant: ["tabular-nums"],
  },
  signOut: {
    alignItems: "center",
    marginTop: 28,
  },
});
