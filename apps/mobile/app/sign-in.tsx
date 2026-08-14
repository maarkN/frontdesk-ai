/**
 * Sign in with the tenant API key (fdk_..., shown once at onboarding).
 * The key is validated against GET /v1/tenants/me and stored in the
 * device Keychain/Keystore via expo-secure-store.
 */
import { Redirect, router } from "expo-router";
import { useState, type ReactElement } from "react";
import { useTranslation } from "react-i18next";
import {
  ActivityIndicator,
  KeyboardAvoidingView,
  Platform,
  StyleSheet,
  Text,
  TextInput,
  View,
} from "react-native";

import { ApiError } from "@frontdesk/shared";

import { PillButton } from "../src/components/ui";
import { useSession } from "../src/session";
import { useTheme } from "../src/theme";

export default function SignInScreen(): ReactElement {
  const theme = useTheme();
  const { t } = useTranslation();
  const { status, signIn } = useSession();
  const [apiKey, setApiKey] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);

  if (status === "signedIn") {
    return <Redirect href="/" />;
  }

  const submit = (): void => {
    if (busy || apiKey.trim() === "") {
      return;
    }
    setBusy(true);
    setError(undefined);
    void signIn(apiKey)
      .then(() => {
        router.replace("/");
      })
      .catch((cause: unknown) => {
        if (cause instanceof ApiError && cause.isUnauthorized) {
          setError(t("auth.invalidKey"));
        } else {
          setError(t("errors.network"));
        }
      })
      .finally(() => {
        setBusy(false);
      });
  };

  return (
    <KeyboardAvoidingView
      style={[styles.screen, { backgroundColor: theme.colors.background }]}
      behavior={Platform.OS === "ios" ? "padding" : undefined}
    >
      <View style={styles.body}>
        <Text style={[styles.title, { color: theme.colors.text }]}>
          {t("common.appName")}
        </Text>
        <Text style={[styles.tagline, { color: theme.colors.textMuted }]}>
          {t("mobile:signIn.tagline")}
        </Text>

        <Text style={[styles.label, { color: theme.colors.textMuted }]}>
          {t("auth.apiKeyLabel")}
        </Text>
        <TextInput
          value={apiKey}
          onChangeText={setApiKey}
          placeholder={t("auth.apiKeyPlaceholder")}
          placeholderTextColor={theme.colors.textMuted}
          autoCapitalize="none"
          autoCorrect={false}
          secureTextEntry
          editable={!busy}
          onSubmitEditing={submit}
          style={[
            styles.input,
            {
              backgroundColor: theme.colors.card,
              borderColor: theme.colors.border,
              color: theme.colors.text,
            },
          ]}
        />
        <Text style={[styles.hint, { color: theme.colors.textMuted }]}>
          {t("mobile:signIn.hint")}
        </Text>

        {error === undefined ? null : (
          <Text style={[styles.error, { color: theme.colors.danger }]}>{error}</Text>
        )}

        {busy ? (
          <ActivityIndicator color={theme.colors.accent} style={styles.busy} />
        ) : (
          <View style={styles.button}>
            <PillButton
              label={t("auth.signIn")}
              onPress={submit}
              disabled={apiKey.trim() === ""}
            />
          </View>
        )}
      </View>
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  screen: {
    flex: 1,
  },
  body: {
    flex: 1,
    justifyContent: "center",
    padding: 24,
  },
  title: {
    fontSize: 28,
    fontWeight: "800",
    textAlign: "center",
  },
  tagline: {
    fontSize: 15,
    marginBottom: 36,
    marginTop: 6,
    textAlign: "center",
  },
  label: {
    fontSize: 13,
    fontWeight: "600",
    marginBottom: 6,
  },
  input: {
    borderRadius: 12,
    borderWidth: 1,
    fontSize: 16,
    padding: 14,
  },
  hint: {
    fontSize: 12,
    marginTop: 8,
  },
  error: {
    fontSize: 13,
    marginTop: 12,
  },
  busy: {
    marginTop: 24,
  },
  button: {
    marginTop: 24,
  },
});
