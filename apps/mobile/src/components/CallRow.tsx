import { Ionicons } from "@expo/vector-icons";
import { Link } from "expo-router";
import type { ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { Pressable, StyleSheet, Text, View } from "react-native";

import type { CallSummary } from "@frontdesk/shared";

import { formatCallDuration, formatDayTime, localeChip } from "../format";
import { currentUiLocale } from "../i18n";
import { callStateLabel, callStateTone } from "../labels";
import { useTheme } from "../theme";
import { Badge } from "./ui";

/** One row in the calls list; links to the call detail. */
export function CallRow({ summary }: { summary: CallSummary }): ReactElement {
  const theme = useTheme();
  const { t } = useTranslation();
  const uiLocale = currentUiLocale();
  const duration = formatCallDuration(summary);

  return (
    <Link href={{ pathname: "/call/[id]", params: { id: summary.callId } }} asChild>
      <Pressable
        accessibilityRole="button"
        style={({ pressed }) => [
          styles.row,
          {
            backgroundColor: theme.colors.card,
            borderColor: theme.colors.border,
            opacity: pressed ? 0.8 : 1,
          },
        ]}
      >
        <View style={styles.main}>
          <Text style={[styles.when, { color: theme.colors.text }]}>
            {summary.startedAt === undefined
              ? t("common.notAvailable")
              : formatDayTime(summary.startedAt, uiLocale)}
          </Text>
          <View style={styles.badges}>
            <Badge
              label={callStateLabel(t, summary.status, summary.endReason)}
              tone={callStateTone(summary)}
            />
            {summary.locale === undefined ? null : (
              <Badge label={localeChip(summary.locale)} tone="neutral" />
            )}
          </View>
        </View>
        <View style={styles.side}>
          {duration === "" ? null : (
            <Text style={[styles.duration, { color: theme.colors.textMuted }]}>{duration}</Text>
          )}
          {summary.recorded ? (
            <Ionicons name="mic" size={14} color={theme.colors.textMuted} />
          ) : null}
          <Ionicons name="chevron-forward" size={16} color={theme.colors.textMuted} />
        </View>
      </Pressable>
    </Link>
  );
}

const styles = StyleSheet.create({
  row: {
    alignItems: "center",
    borderRadius: 14,
    borderWidth: StyleSheet.hairlineWidth,
    flexDirection: "row",
    justifyContent: "space-between",
    marginBottom: 8,
    padding: 14,
  },
  main: {
    flex: 1,
    gap: 6,
  },
  when: {
    fontSize: 15,
    fontWeight: "600",
  },
  badges: {
    flexDirection: "row",
    gap: 6,
  },
  side: {
    alignItems: "center",
    flexDirection: "row",
    gap: 6,
  },
  duration: {
    fontSize: 13,
    fontVariant: ["tabular-nums"],
  },
});
