import type { ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";

import type { Appointment } from "@frontdesk/shared";

import { formatDayTime, formatTime } from "../format";
import { currentUiLocale } from "../i18n";
import { useTheme } from "../theme";
import { Card } from "./ui";

/** Compact upcoming-appointment row for the Home screen. */
export function AppointmentRow({ appointment }: { appointment: Appointment }): ReactElement {
  const theme = useTheme();
  const uiLocale = currentUiLocale();
  return (
    <Card style={styles.card}>
      <View style={styles.textColumn}>
        <Text style={[styles.service, { color: theme.colors.text }]} numberOfLines={1}>
          {appointment.service}
        </Text>
        <Text style={[styles.customer, { color: theme.colors.textMuted }]} numberOfLines={1}>
          {appointment.customerName}
          {appointment.postalCode === undefined ? "" : ` · ${appointment.postalCode}`}
        </Text>
      </View>
      <Text style={[styles.when, { color: theme.colors.accent }]}>
        {formatDayTime(appointment.startsAt, uiLocale)}
        {" – "}
        {formatTime(appointment.endsAt, uiLocale)}
      </Text>
    </Card>
  );
}

const styles = StyleSheet.create({
  card: {
    gap: 6,
    marginBottom: 8,
  },
  textColumn: {
    gap: 2,
  },
  service: {
    fontSize: 15,
    fontWeight: "600",
  },
  customer: {
    fontSize: 13,
  },
  when: {
    fontSize: 13,
    fontWeight: "600",
  },
});
