/**
 * Call detail — folded snapshot of one call: header facts, the recording
 * player (expo-audio) and the transcript as heard. Push taps on post-call
 * summaries land here (data.callId).
 */
import { Redirect, useLocalSearchParams } from "expo-router";
import type { ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { ScrollView, StyleSheet, Text, View } from "react-native";

import { ApiError } from "@frontdesk/shared";

import { AudioPlayer } from "../../src/components/AudioPlayer";
import { TranscriptView } from "../../src/components/TranscriptView";
import { Badge, Card, ErrorView, LoadingView, SectionHeader } from "../../src/components/ui";
import { formatCallDuration, formatDayTime, localeChip } from "../../src/format";
import { currentUiLocale } from "../../src/i18n";
import { callStateLabel, callStateTone } from "../../src/labels";
import { useCallQuery } from "../../src/queries";
import { useSession } from "../../src/session";
import { useTheme } from "../../src/theme";

export default function CallDetailScreen(): ReactElement {
  const theme = useTheme();
  const { t } = useTranslation();
  const { status } = useSession();
  const params = useLocalSearchParams();
  const rawId: string | string[] | undefined = params["id"];
  const callId =
    typeof rawId === "string" ? rawId : Array.isArray(rawId) ? (rawId[0] ?? "") : "";
  const callQuery = useCallQuery(callId);
  const uiLocale = currentUiLocale();

  if (status === "signedOut") {
    return <Redirect href="/sign-in" />;
  }
  if (status === "loading" || callQuery.isPending) {
    return <LoadingView />;
  }
  if (callQuery.isError) {
    const notFound = callQuery.error instanceof ApiError && callQuery.error.isNotFound;
    return (
      <ErrorView
        message={notFound ? t("errors.notFound") : t("errors.network")}
        onRetry={
          notFound
            ? undefined
            : () => {
                void callQuery.refetch();
              }
        }
      />
    );
  }

  const { snapshot, recordingUrl } = callQuery.data;
  const duration = formatCallDuration(snapshot);

  return (
    <ScrollView
      style={{ backgroundColor: theme.colors.background }}
      contentContainerStyle={styles.content}
    >
      <Card>
        <View style={styles.badgeRow}>
          <Badge
            label={callStateLabel(t, snapshot.status, snapshot.endReason)}
            tone={callStateTone(snapshot)}
          />
          {snapshot.locale === undefined ? null : (
            <Badge label={localeChip(snapshot.locale)} tone="neutral" />
          )}
          {snapshot.recording ? <Badge label={t("calls.recorded")} tone="accent" /> : null}
        </View>
        <FactRow
          label={t("calls.startedAt")}
          value={
            snapshot.startedAt === undefined
              ? t("common.notAvailable")
              : formatDayTime(snapshot.startedAt, uiLocale)
          }
        />
        <FactRow
          label={t("calls.duration")}
          value={duration === "" ? t("common.notAvailable") : duration}
        />
        <FactRow
          label={t("calls.language")}
          value={
            snapshot.localeHistory === undefined || snapshot.localeHistory.length === 0
              ? snapshot.locale === undefined
                ? t("common.notAvailable")
                : localeChip(snapshot.locale)
              : snapshot.localeHistory.map(localeChip).join(" → ")
          }
        />
        {snapshot.consentCaptured ? (
          <Text
            style={[
              styles.consent,
              {
                color: snapshot.consentGranted
                  ? theme.colors.success
                  : theme.colors.warning,
              },
            ]}
          >
            {snapshot.consentGranted
              ? t("calls.consentGranted")
              : t("calls.consentDenied")}
          </Text>
        ) : null}
      </Card>

      {recordingUrl === undefined ? null : (
        <>
          <SectionHeader title={t("calls.playRecording")} />
          <AudioPlayer uri={recordingUrl} />
        </>
      )}

      <TranscriptView transcript={snapshot.transcript} />
    </ScrollView>
  );
}

function FactRow({ label, value }: { label: string; value: string }): ReactElement {
  const theme = useTheme();
  return (
    <View style={styles.factRow}>
      <Text style={[styles.factLabel, { color: theme.colors.textMuted }]}>{label}</Text>
      <Text style={[styles.factValue, { color: theme.colors.text }]}>{value}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  content: {
    padding: 16,
    paddingBottom: 32,
  },
  badgeRow: {
    flexDirection: "row",
    gap: 6,
    marginBottom: 10,
  },
  factRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    paddingVertical: 4,
  },
  factLabel: {
    fontSize: 14,
  },
  factValue: {
    fontSize: 14,
    fontWeight: "600",
  },
  consent: {
    fontSize: 13,
    marginTop: 8,
  },
});
