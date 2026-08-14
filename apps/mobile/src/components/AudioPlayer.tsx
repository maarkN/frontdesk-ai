import { Ionicons } from "@expo/vector-icons";
import { useAudioPlayer, useAudioPlayerStatus } from "expo-audio";
import type { ReactElement } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";

import { formatClock } from "../format";
import { useTheme } from "../theme";
import { Card } from "./ui";

const SKIP_SECONDS = 15;

/**
 * Recording player (expo-audio — the SDK 54 audio API; expo-av is legacy).
 * Play/pause, ±15s skips and a progress bar; streams the call's recordingUrl.
 */
export function AudioPlayer({ uri }: { uri: string }): ReactElement {
  const theme = useTheme();
  const player = useAudioPlayer({ uri });
  const status = useAudioPlayerStatus(player);

  const duration = status.duration > 0 ? status.duration : 0;
  const progress = duration > 0 ? Math.min(status.currentTime / duration, 1) : 0;
  const progressWidth: `${number}%` = `${progress * 100}%`;

  const togglePlayback = (): void => {
    if (status.playing) {
      player.pause();
      return;
    }
    if (status.didJustFinish || (duration > 0 && status.currentTime >= duration)) {
      void player.seekTo(0);
    }
    player.play();
  };

  const skip = (deltaSeconds: number): void => {
    const target = Math.max(0, Math.min(status.currentTime + deltaSeconds, duration));
    void player.seekTo(target);
  };

  return (
    <Card>
      <View style={styles.controls}>
        <Pressable
          accessibilityRole="button"
          onPress={() => {
            skip(-SKIP_SECONDS);
          }}
          hitSlop={8}
        >
          <Ionicons name="play-back" size={22} color={theme.colors.textMuted} />
        </Pressable>
        <Pressable
          accessibilityRole="button"
          onPress={togglePlayback}
          style={[styles.playButton, { backgroundColor: theme.colors.accent }]}
        >
          <Ionicons
            name={status.playing ? "pause" : "play"}
            size={24}
            color={theme.colors.onAccent}
          />
        </Pressable>
        <Pressable
          accessibilityRole="button"
          onPress={() => {
            skip(SKIP_SECONDS);
          }}
          hitSlop={8}
        >
          <Ionicons name="play-forward" size={22} color={theme.colors.textMuted} />
        </Pressable>
      </View>
      <View style={[styles.track, { backgroundColor: theme.colors.border }]}>
        <View
          style={[
            styles.trackFill,
            { backgroundColor: theme.colors.accent, width: progressWidth },
          ]}
        />
      </View>
      <View style={styles.times}>
        <Text style={[styles.time, { color: theme.colors.textMuted }]}>
          {formatClock(status.currentTime)}
        </Text>
        <Text style={[styles.time, { color: theme.colors.textMuted }]}>
          {formatClock(duration)}
        </Text>
      </View>
    </Card>
  );
}

const styles = StyleSheet.create({
  controls: {
    alignItems: "center",
    flexDirection: "row",
    gap: 28,
    justifyContent: "center",
  },
  playButton: {
    alignItems: "center",
    borderRadius: 999,
    height: 52,
    justifyContent: "center",
    width: 52,
  },
  track: {
    borderRadius: 999,
    height: 4,
    marginTop: 14,
    overflow: "hidden",
  },
  trackFill: {
    borderRadius: 999,
    height: 4,
  },
  times: {
    flexDirection: "row",
    justifyContent: "space-between",
    marginTop: 6,
  },
  time: {
    fontSize: 12,
    fontVariant: ["tabular-nums"],
  },
});
