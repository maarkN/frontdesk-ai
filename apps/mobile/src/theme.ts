import { useColorScheme } from "react-native";

export interface ThemeColors {
  background: string;
  card: string;
  text: string;
  textMuted: string;
  border: string;
  accent: string;
  onAccent: string;
  accentSoft: string;
  danger: string;
  dangerSoft: string;
  warning: string;
  warningSoft: string;
  success: string;
  successSoft: string;
  /** Chat bubble backgrounds for the transcript. */
  callerBubble: string;
  agentBubble: string;
}

export interface Theme {
  dark: boolean;
  colors: ThemeColors;
}

const light: Theme = {
  dark: false,
  colors: {
    background: "#F4F6F8",
    card: "#FFFFFF",
    text: "#101828",
    textMuted: "#667085",
    border: "#E4E7EC",
    accent: "#1D4ED8",
    onAccent: "#FFFFFF",
    accentSoft: "#EFF4FF",
    danger: "#B42318",
    dangerSoft: "#FEF3F2",
    warning: "#B54708",
    warningSoft: "#FFFAEB",
    success: "#067647",
    successSoft: "#ECFDF3",
    callerBubble: "#EAECF0",
    agentBubble: "#EFF4FF",
  },
};

const dark: Theme = {
  dark: true,
  colors: {
    background: "#0C111D",
    card: "#161B26",
    text: "#F5F5F6",
    textMuted: "#94969C",
    border: "#1F242F",
    accent: "#528BFF",
    onAccent: "#FFFFFF",
    accentSoft: "#1B2A50",
    danger: "#F97066",
    dangerSoft: "#3A1A18",
    warning: "#FDB022",
    warningSoft: "#3A2A10",
    success: "#47CD89",
    successSoft: "#113222",
    callerBubble: "#1F242F",
    agentBubble: "#1B2A50",
  },
};

/** Light/dark theme following the system appearance. */
export function useTheme(): Theme {
  return useColorScheme() === "dark" ? dark : light;
}
