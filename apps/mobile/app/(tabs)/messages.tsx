/**
 * Messages — structured notes the AI took, split into "to handle" and
 * "handled". Quick actions per card: call back (tel:) and mark handled.
 */
import type { ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { RefreshControl, SectionList, StyleSheet, View } from "react-native";

import type { Message } from "@frontdesk/shared";

import { MessageCard } from "../../src/components/MessageCard";
import { EmptyView, ErrorView, LoadingView, SectionHeader } from "../../src/components/ui";
import { useHandledMessages } from "../../src/handled";
import { useMessagesQuery } from "../../src/queries";
import { useTheme } from "../../src/theme";

interface MessageSection {
  title: string;
  data: Message[];
}

const URGENCY_RANK = { emergency: 0, high: 1, normal: 2, low: 3 } as const;

export default function MessagesScreen(): ReactElement {
  const theme = useTheme();
  const { t } = useTranslation();
  const { isHandled, markHandled } = useHandledMessages();
  const messagesQuery = useMessagesQuery();

  if (messagesQuery.isPending) {
    return <LoadingView />;
  }
  if (messagesQuery.isError) {
    return (
      <ErrorView
        message={t("errors.network")}
        onRetry={() => {
          void messagesQuery.refetch();
        }}
      />
    );
  }

  const open = messagesQuery.data
    .filter((m) => !isHandled(m.id))
    .sort(
      (a, b) =>
        URGENCY_RANK[a.urgency] - URGENCY_RANK[b.urgency] ||
        b.createdAt.localeCompare(a.createdAt),
    );
  const handled = messagesQuery.data.filter((m) => isHandled(m.id));

  const sections: MessageSection[] = [];
  if (open.length > 0) {
    sections.push({ title: t("mobile:messageActions.openSection"), data: open });
  }
  if (handled.length > 0) {
    sections.push({ title: t("mobile:messageActions.handledSection"), data: handled });
  }

  return (
    <SectionList
      style={{ backgroundColor: theme.colors.background }}
      contentContainerStyle={styles.content}
      sections={sections}
      keyExtractor={(message) => message.id}
      stickySectionHeadersEnabled={false}
      renderSectionHeader={({ section }) => <SectionHeader title={section.title} />}
      renderItem={({ item }) => (
        <View style={styles.item}>
          <MessageCard
            message={item}
            handled={isHandled(item.id)}
            onMarkHandled={markHandled}
          />
        </View>
      )}
      ListEmptyComponent={<EmptyView message={t("messages.empty")} />}
      refreshControl={
        <RefreshControl
          refreshing={messagesQuery.isRefetching}
          onRefresh={() => {
            void messagesQuery.refetch();
          }}
          tintColor={theme.colors.accent}
        />
      }
    />
  );
}

const styles = StyleSheet.create({
  content: {
    flexGrow: 1,
    padding: 16,
    paddingTop: 0,
  },
  item: {
    marginBottom: 8,
  },
});
