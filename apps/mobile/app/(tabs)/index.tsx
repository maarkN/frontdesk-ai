/**
 * Home — the owner's day at a glance: urgent messages first (the "act now"
 * list), today's AI-answered call stats, then the next appointments.
 */
import { Link } from "expo-router";
import { useEffect, type ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { RefreshControl, ScrollView, StyleSheet, Text, View } from "react-native";

import { ApiError, type Message } from "@frontdesk/shared";

import { AppointmentRow } from "../../src/components/AppointmentRow";
import { MessageCard } from "../../src/components/MessageCard";
import { Card, EmptyView, ErrorView, LoadingView, SectionHeader } from "../../src/components/ui";
import { isSameLocalDay } from "../../src/format";
import { useHandledMessages } from "../../src/handled";
import {
  useAppointmentsQuery,
  useCallsQuery,
  useMessagesQuery,
  useTenantQuery,
} from "../../src/queries";
import { useSession } from "../../src/session";
import { useTheme } from "../../src/theme";

const URGENT_LIMIT = 3;
const UPCOMING_LIMIT = 3;

export default function HomeScreen(): ReactElement {
  const theme = useTheme();
  const { t } = useTranslation();
  const { signOut } = useSession();
  const { isHandled, markHandled } = useHandledMessages();

  const tenantQuery = useTenantQuery();
  const callsQuery = useCallsQuery();
  const messagesQuery = useMessagesQuery();
  const appointmentsQuery = useAppointmentsQuery();

  // A revoked/rotated key turns every request into a 401 — drop the session.
  useEffect(() => {
    if (tenantQuery.error instanceof ApiError && tenantQuery.error.isUnauthorized) {
      void signOut();
    }
  }, [tenantQuery.error, signOut]);

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

  const now = new Date();
  const calls = callsQuery.data ?? [];
  const todayCalls = calls.filter(
    (c) => c.startedAt !== undefined && isSameLocalDay(c.startedAt, now),
  );
  const todayMinutes = todayCalls.reduce((total, c) => total + c.callMinutes, 0);

  const messages = messagesQuery.data ?? [];
  const todayMessages = messages.filter((m) => isSameLocalDay(m.createdAt, now));
  const urgent: Message[] = messages
    .filter(
      (m) => (m.urgency === "emergency" || m.urgency === "high") && !isHandled(m.id),
    )
    .slice(0, URGENT_LIMIT);

  const upcoming = (appointmentsQuery.data ?? [])
    .filter((a) => new Date(a.startsAt).getTime() >= now.getTime())
    .sort((a, b) => a.startsAt.localeCompare(b.startsAt))
    .slice(0, UPCOMING_LIMIT);

  const refreshing =
    callsQuery.isRefetching || messagesQuery.isRefetching || appointmentsQuery.isRefetching;
  const onRefresh = (): void => {
    void callsQuery.refetch();
    void messagesQuery.refetch();
    void appointmentsQuery.refetch();
    void tenantQuery.refetch();
  };

  return (
    <ScrollView
      style={{ backgroundColor: theme.colors.background }}
      contentContainerStyle={styles.content}
      refreshControl={
        <RefreshControl
          refreshing={refreshing}
          onRefresh={onRefresh}
          tintColor={theme.colors.accent}
        />
      }
    >
      <Text style={[styles.greeting, { color: theme.colors.text }]}>
        {t("mobile:home.greeting", { name: tenantQuery.data.name })}
      </Text>

      <SectionHeader title={t("mobile:home.urgent")} />
      {urgent.length === 0 ? (
        <Card>
          <Text style={[styles.allClear, { color: theme.colors.success }]}>
            {t("mobile:home.allClear")}
          </Text>
        </Card>
      ) : (
        <View style={styles.stack}>
          {urgent.map((message) => (
            <MessageCard
              key={message.id}
              message={message}
              handled={false}
              onMarkHandled={markHandled}
            />
          ))}
        </View>
      )}

      <SectionHeader title={t("mobile:home.today")} />
      <View style={styles.statsRow}>
        <StatTile label={t("mobile:home.answered")} value={String(todayCalls.length)} />
        <StatTile label={t("mobile:home.messagesTaken")} value={String(todayMessages.length)} />
        <StatTile label={t("mobile:home.minutes")} value={String(todayMinutes)} />
      </View>

      <View style={styles.upcomingHeader}>
        <SectionHeader title={t("mobile:home.upcoming")} />
        <Link href="/calls" style={[styles.seeAll, { color: theme.colors.accent }]}>
          {t("mobile:home.seeAll")}
        </Link>
      </View>
      {upcoming.length === 0 ? (
        <EmptyView message={t("mobile:home.noUpcoming")} />
      ) : (
        upcoming.map((appointment) => (
          <AppointmentRow key={appointment.id} appointment={appointment} />
        ))
      )}
    </ScrollView>
  );
}

function StatTile({ label, value }: { label: string; value: string }): ReactElement {
  const theme = useTheme();
  return (
    <Card style={styles.statTile}>
      <Text style={[styles.statValue, { color: theme.colors.text }]}>{value}</Text>
      <Text style={[styles.statLabel, { color: theme.colors.textMuted }]}>{label}</Text>
    </Card>
  );
}

const styles = StyleSheet.create({
  content: {
    padding: 16,
    paddingBottom: 32,
  },
  greeting: {
    fontSize: 22,
    fontWeight: "700",
  },
  stack: {
    gap: 8,
  },
  allClear: {
    fontSize: 14,
    fontWeight: "600",
  },
  statsRow: {
    flexDirection: "row",
    gap: 8,
  },
  statTile: {
    alignItems: "center",
    flex: 1,
  },
  statValue: {
    fontSize: 24,
    fontVariant: ["tabular-nums"],
    fontWeight: "700",
  },
  statLabel: {
    fontSize: 12,
    marginTop: 2,
    textAlign: "center",
  },
  upcomingHeader: {
    alignItems: "baseline",
    flexDirection: "row",
    justifyContent: "space-between",
  },
  seeAll: {
    fontSize: 13,
    fontWeight: "600",
  },
});
