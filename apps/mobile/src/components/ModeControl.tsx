import type { ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { ActivityIndicator, Pressable, StyleSheet, Text, View } from "react-native";

import type { Mode, Tenant } from "@frontdesk/shared";

import { useUpdateModeMutation } from "../queries";
import { useTheme } from "../theme";
import { Card } from "./ui";

const OVERFLOW_CHOICES = [15, 20, 30, 45] as const;
const DEFAULT_OVERFLOW_SECONDS = 20;

/**
 * The product's #1 control (RF1/RF4): overflow — the AI answers only when
 * nobody picks up within N seconds — vs always-AI. Segmented toggle plus the
 * ring-time picker when in overflow mode.
 */
export function ModeControl({ tenant }: { tenant: Tenant }): ReactElement {
  const theme = useTheme();
  const { t } = useTranslation();
  const mutation = useUpdateModeMutation();

  const overflowSeconds = tenant.overflowSeconds ?? DEFAULT_OVERFLOW_SECONDS;

  const setMode = (mode: Mode): void => {
    if (mode === tenant.mode || mutation.isPending) {
      return;
    }
    mutation.mutate(
      mode === "overflow" ? { mode, overflowSeconds } : { mode },
    );
  };

  const setOverflowSeconds = (seconds: number): void => {
    if (mutation.isPending) {
      return;
    }
    mutation.mutate({ mode: "overflow", overflowSeconds: seconds });
  };

  const segment = (mode: Mode, label: string): ReactElement => {
    const active = tenant.mode === mode;
    return (
      <Pressable
        accessibilityRole="button"
        accessibilityState={{ selected: active }}
        onPress={() => {
          setMode(mode);
        }}
        style={[
          styles.segment,
          active
            ? { backgroundColor: theme.colors.accent }
            : { backgroundColor: "transparent" },
        ]}
      >
        <Text
          style={[
            styles.segmentText,
            { color: active ? theme.colors.onAccent : theme.colors.textMuted },
          ]}
        >
          {label}
        </Text>
      </Pressable>
    );
  };

  return (
    <Card>
      <View style={styles.titleRow}>
        <Text style={[styles.title, { color: theme.colors.text }]}>{t("mode.title")}</Text>
        {mutation.isPending ? <ActivityIndicator size="small" color={theme.colors.accent} /> : null}
      </View>
      <View style={[styles.segments, { backgroundColor: theme.colors.border }]}>
        {segment("overflow", t("mode.overflow"))}
        {segment("always_ai", t("mode.alwaysAi"))}
      </View>
      <Text style={[styles.hint, { color: theme.colors.textMuted }]}>
        {tenant.mode === "overflow"
          ? t("mode.overflowHint", { seconds: overflowSeconds })
          : t("mode.alwaysAiHint")}
      </Text>
      {tenant.mode === "overflow" ? (
        <View style={styles.secondsBlock}>
          <Text style={[styles.secondsLabel, { color: theme.colors.textMuted }]}>
            {t("mode.overflowSecondsLabel")}
          </Text>
          <View style={styles.secondsRow}>
            {OVERFLOW_CHOICES.map((seconds) => {
              const active = overflowSeconds === seconds;
              return (
                <Pressable
                  key={seconds}
                  accessibilityRole="button"
                  accessibilityState={{ selected: active }}
                  onPress={() => {
                    setOverflowSeconds(seconds);
                  }}
                  style={[
                    styles.secondsChip,
                    {
                      backgroundColor: active ? theme.colors.accentSoft : "transparent",
                      borderColor: active ? theme.colors.accent : theme.colors.border,
                    },
                  ]}
                >
                  <Text
                    style={[
                      styles.secondsChipText,
                      { color: active ? theme.colors.accent : theme.colors.textMuted },
                    ]}
                  >
                    {seconds}s
                  </Text>
                </Pressable>
              );
            })}
          </View>
        </View>
      ) : null}
      {mutation.isError ? (
        <Text style={[styles.error, { color: theme.colors.danger }]}>
          {t("errors.generic")}
        </Text>
      ) : null}
    </Card>
  );
}

const styles = StyleSheet.create({
  titleRow: {
    alignItems: "center",
    flexDirection: "row",
    justifyContent: "space-between",
    marginBottom: 10,
  },
  title: {
    fontSize: 16,
    fontWeight: "700",
  },
  segments: {
    borderRadius: 10,
    flexDirection: "row",
    padding: 3,
  },
  segment: {
    alignItems: "center",
    borderRadius: 8,
    flex: 1,
    paddingVertical: 8,
  },
  segmentText: {
    fontSize: 14,
    fontWeight: "600",
  },
  hint: {
    fontSize: 13,
    marginTop: 10,
  },
  secondsBlock: {
    marginTop: 12,
  },
  secondsLabel: {
    fontSize: 13,
    marginBottom: 8,
  },
  secondsRow: {
    flexDirection: "row",
    gap: 8,
  },
  secondsChip: {
    borderRadius: 999,
    borderWidth: 1,
    paddingHorizontal: 14,
    paddingVertical: 6,
  },
  secondsChipText: {
    fontSize: 13,
    fontWeight: "600",
  },
  error: {
    fontSize: 13,
    marginTop: 10,
  },
});
