import type { ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { StyleSheet, Text, View } from "react-native";

import type { TranscriptEntry } from "@frontdesk/shared";

import { formatTime } from "../format";
import { currentUiLocale } from "../i18n";
import { useTheme } from "../theme";
import { EmptyView, SectionHeader } from "./ui";

/**
 * Chat-style transcript. Agent turns are what the caller actually HEARD
 * (PlayoutTracker truncation) — an interrupted turn is marked with "—".
 * Caller text arrives already PII-redacted; redacted spans are counted.
 */
export function TranscriptView({
  transcript,
}: {
  transcript: readonly TranscriptEntry[] | undefined;
}): ReactElement {
  const theme = useTheme();
  const { t } = useTranslation();
  const uiLocale = currentUiLocale();

  return (
    <View>
      <SectionHeader title={t("transcript.title")} />
      {transcript === undefined || transcript.length === 0 ? (
        <EmptyView message={t("transcript.empty")} />
      ) : (
        transcript.map((entry) => {
          const isAgent = entry.speaker === "agent";
          const redactionCount = entry.redactions?.length ?? 0;
          return (
            <View
              key={entry.seq}
              style={[styles.bubbleRow, isAgent ? styles.agentRow : styles.callerRow]}
            >
              <View
                style={[
                  styles.bubble,
                  {
                    backgroundColor: isAgent
                      ? theme.colors.agentBubble
                      : theme.colors.callerBubble,
                  },
                ]}
              >
                <Text style={[styles.speaker, { color: theme.colors.textMuted }]}>
                  {isAgent ? t("transcript.agent") : t("transcript.caller")}
                  {" · "}
                  {formatTime(entry.ts, uiLocale)}
                </Text>
                <Text style={[styles.text, { color: theme.colors.text }]}>
                  {entry.text}
                  {entry.interrupted === true ? " —" : ""}
                </Text>
                {entry.interrupted === true ? (
                  <Text style={[styles.note, { color: theme.colors.warning }]}>
                    {t("transcript.interrupted")}
                  </Text>
                ) : null}
                {redactionCount > 0 ? (
                  <Text style={[styles.note, { color: theme.colors.textMuted }]}>
                    {redactionCount} × {t("transcript.redacted")}
                  </Text>
                ) : null}
              </View>
            </View>
          );
        })
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  bubbleRow: {
    flexDirection: "row",
    marginBottom: 8,
  },
  agentRow: {
    justifyContent: "flex-end",
  },
  callerRow: {
    justifyContent: "flex-start",
  },
  bubble: {
    borderRadius: 14,
    maxWidth: "85%",
    padding: 10,
  },
  speaker: {
    fontSize: 11,
    fontWeight: "700",
    marginBottom: 3,
  },
  text: {
    fontSize: 14,
    lineHeight: 20,
  },
  note: {
    fontSize: 11,
    fontStyle: "italic",
    marginTop: 4,
  },
});
