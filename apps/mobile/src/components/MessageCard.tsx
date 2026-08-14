import type { ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { Linking, StyleSheet, Text, View } from "react-native";

import type { Message } from "@frontdesk/shared";

import { formatDayTime, formatPhone } from "../format";
import { currentUiLocale } from "../i18n";
import { urgencyLabel, urgencyTone } from "../labels";
import { useTheme } from "../theme";
import { Badge, Card, PillButton } from "./ui";

/**
 * A structured message with the two quick actions the owner needs in the
 * field: call back (tel:) and mark handled (device-local until the API grows
 * a handled flag).
 */
export function MessageCard({
  message,
  handled,
  onMarkHandled,
}: {
  message: Message;
  handled: boolean;
  onMarkHandled: (id: string) => void;
}): ReactElement {
  const theme = useTheme();
  const { t } = useTranslation();
  const uiLocale = currentUiLocale();

  const callBack = (): void => {
    void Linking.openURL(`tel:${message.callbackPhone}`).catch(() => {
      // No dialer (simulator) — nothing sensible to do.
    });
  };

  return (
    <Card style={handled ? styles.handledCard : undefined}>
      <View style={styles.header}>
        <Text style={[styles.who, { color: theme.colors.text }]} numberOfLines={1}>
          {message.who}
        </Text>
        <Badge label={urgencyLabel(t, message.urgency)} tone={urgencyTone(message.urgency)} />
      </View>
      <Text style={[styles.what, { color: theme.colors.text }]}>{message.what}</Text>
      <Text style={[styles.meta, { color: theme.colors.textMuted }]}>
        {formatPhone(message.callbackPhone)} · {formatDayTime(message.createdAt, uiLocale)}
      </Text>
      <View style={styles.actions}>
        <PillButton label={t("mobile:messageActions.callBack")} onPress={callBack} />
        {handled ? (
          <Badge label={t("mobile:messageActions.handled")} tone="success" />
        ) : (
          <PillButton
            label={t("mobile:messageActions.markHandled")}
            tone="neutral"
            onPress={() => {
              onMarkHandled(message.id);
            }}
          />
        )}
      </View>
    </Card>
  );
}

const styles = StyleSheet.create({
  handledCard: {
    opacity: 0.6,
  },
  header: {
    alignItems: "center",
    flexDirection: "row",
    gap: 8,
    justifyContent: "space-between",
  },
  who: {
    flex: 1,
    fontSize: 15,
    fontWeight: "700",
  },
  what: {
    fontSize: 14,
    marginTop: 6,
  },
  meta: {
    fontSize: 13,
    marginTop: 6,
  },
  actions: {
    alignItems: "center",
    flexDirection: "row",
    gap: 10,
    marginTop: 12,
  },
});
