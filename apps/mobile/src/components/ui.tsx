/** Small building blocks shared by every screen. Plain StyleSheet, themed. */
import type { ReactElement, ReactNode } from "react";
import {
  ActivityIndicator,
  Pressable,
  StyleSheet,
  Text,
  View,
  type StyleProp,
  type ViewStyle,
} from "react-native";
import { useTranslation } from "react-i18next";

import { useTheme, type Theme } from "../theme";

export function Card({
  children,
  style,
}: {
  children: ReactNode;
  style?: StyleProp<ViewStyle>;
}): ReactElement {
  const theme = useTheme();
  return (
    <View
      style={[
        styles.card,
        { backgroundColor: theme.colors.card, borderColor: theme.colors.border },
        style,
      ]}
    >
      {children}
    </View>
  );
}

export function SectionHeader({ title }: { title: string }): ReactElement {
  const theme = useTheme();
  return (
    <Text style={[styles.sectionHeader, { color: theme.colors.textMuted }]}>
      {title.toUpperCase()}
    </Text>
  );
}

export type BadgeTone = "neutral" | "accent" | "warning" | "danger" | "success";

function badgeColors(theme: Theme, tone: BadgeTone): { bg: string; fg: string } {
  const palette: Record<BadgeTone, { bg: string; fg: string }> = {
    accent: { bg: theme.colors.accentSoft, fg: theme.colors.accent },
    warning: { bg: theme.colors.warningSoft, fg: theme.colors.warning },
    danger: { bg: theme.colors.dangerSoft, fg: theme.colors.danger },
    success: { bg: theme.colors.successSoft, fg: theme.colors.success },
    neutral: { bg: theme.colors.border, fg: theme.colors.textMuted },
  };
  return palette[tone];
}

export function Badge({
  label,
  tone = "neutral",
}: {
  label: string;
  tone?: BadgeTone;
}): ReactElement {
  const theme = useTheme();
  const { bg, fg } = badgeColors(theme, tone);
  return (
    <View style={[styles.badge, { backgroundColor: bg }]}>
      <Text style={[styles.badgeText, { color: fg }]}>{label}</Text>
    </View>
  );
}

export function PillButton({
  label,
  onPress,
  tone = "accent",
  disabled = false,
}: {
  label: string;
  onPress: () => void;
  tone?: "accent" | "neutral" | "danger";
  disabled?: boolean;
}): ReactElement {
  const theme = useTheme();
  const background =
    tone === "accent"
      ? theme.colors.accent
      : tone === "danger"
        ? theme.colors.dangerSoft
        : theme.colors.border;
  const foreground =
    tone === "accent"
      ? theme.colors.onAccent
      : tone === "danger"
        ? theme.colors.danger
        : theme.colors.text;
  return (
    <Pressable
      accessibilityRole="button"
      onPress={onPress}
      disabled={disabled}
      style={({ pressed }) => [
        styles.pill,
        { backgroundColor: background, opacity: disabled ? 0.5 : pressed ? 0.8 : 1 },
      ]}
    >
      <Text style={[styles.pillText, { color: foreground }]}>{label}</Text>
    </Pressable>
  );
}

export function LoadingView(): ReactElement {
  const theme = useTheme();
  const { t } = useTranslation();
  return (
    <View style={styles.centered}>
      <ActivityIndicator color={theme.colors.accent} />
      <Text style={[styles.centeredText, { color: theme.colors.textMuted }]}>
        {t("common.loading")}
      </Text>
    </View>
  );
}

export function ErrorView({
  message,
  onRetry,
}: {
  message: string;
  onRetry?: (() => void) | undefined;
}): ReactElement {
  const theme = useTheme();
  const { t } = useTranslation();
  return (
    <View style={styles.centered}>
      <Text style={[styles.centeredText, { color: theme.colors.danger }]}>{message}</Text>
      {onRetry === undefined ? null : (
        <PillButton label={t("common.retry")} onPress={onRetry} tone="neutral" />
      )}
    </View>
  );
}

export function EmptyView({ message }: { message: string }): ReactElement {
  const theme = useTheme();
  return (
    <View style={styles.centered}>
      <Text style={[styles.centeredText, { color: theme.colors.textMuted }]}>{message}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  card: {
    borderRadius: 14,
    borderWidth: StyleSheet.hairlineWidth,
    padding: 14,
  },
  sectionHeader: {
    fontSize: 12,
    fontWeight: "700",
    letterSpacing: 0.8,
    marginBottom: 8,
    marginTop: 20,
  },
  badge: {
    alignSelf: "flex-start",
    borderRadius: 999,
    paddingHorizontal: 8,
    paddingVertical: 3,
  },
  badgeText: {
    fontSize: 11,
    fontWeight: "700",
  },
  pill: {
    alignItems: "center",
    borderRadius: 999,
    paddingHorizontal: 16,
    paddingVertical: 9,
  },
  pillText: {
    fontSize: 14,
    fontWeight: "600",
  },
  centered: {
    alignItems: "center",
    flex: 1,
    gap: 12,
    justifyContent: "center",
    padding: 32,
  },
  centeredText: {
    fontSize: 14,
    textAlign: "center",
  },
});
