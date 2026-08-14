/** Calls — every call the AI touched, newest first; tap for the detail. */
import type { ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { FlatList, RefreshControl, StyleSheet } from "react-native";

import { CallRow } from "../../src/components/CallRow";
import { EmptyView, ErrorView, LoadingView } from "../../src/components/ui";
import { useCallsQuery } from "../../src/queries";
import { useTheme } from "../../src/theme";

export default function CallsScreen(): ReactElement {
  const theme = useTheme();
  const { t } = useTranslation();
  const callsQuery = useCallsQuery();

  if (callsQuery.isPending) {
    return <LoadingView />;
  }
  if (callsQuery.isError) {
    return (
      <ErrorView
        message={t("errors.network")}
        onRetry={() => {
          void callsQuery.refetch();
        }}
      />
    );
  }

  return (
    <FlatList
      style={{ backgroundColor: theme.colors.background }}
      contentContainerStyle={styles.content}
      data={callsQuery.data}
      keyExtractor={(summary) => summary.callId}
      renderItem={({ item }) => <CallRow summary={item} />}
      ListEmptyComponent={<EmptyView message={t("calls.empty")} />}
      refreshControl={
        <RefreshControl
          refreshing={callsQuery.isRefetching}
          onRefresh={() => {
            void callsQuery.refetch();
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
  },
});
