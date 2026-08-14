/** Minimal recording player over the presigned recordingUrl. */
import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

function clock(seconds: number): string {
  if (!Number.isFinite(seconds)) {
    return "0:00";
  }
  const s = Math.floor(seconds);
  return `${Math.floor(s / 60)}:${(s % 60).toString().padStart(2, "0")}`;
}

export function AudioPlayer({ src }: { src: string }) {
  const { t } = useTranslation();
  const ref = useRef<HTMLAudioElement>(null);
  const [playing, setPlaying] = useState(false);
  const [time, setTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    const el = ref.current;
    if (el === null) {
      return;
    }
    const onTime = () => setTime(el.currentTime);
    const onMeta = () => setDuration(el.duration);
    const onEnd = () => setPlaying(false);
    const onErr = () => setFailed(true);
    el.addEventListener("timeupdate", onTime);
    el.addEventListener("loadedmetadata", onMeta);
    el.addEventListener("ended", onEnd);
    el.addEventListener("error", onErr);
    return () => {
      el.removeEventListener("timeupdate", onTime);
      el.removeEventListener("loadedmetadata", onMeta);
      el.removeEventListener("ended", onEnd);
      el.removeEventListener("error", onErr);
    };
  }, []);

  if (failed) {
    return <p className="text-sm text-muted">{t("app:audio.notSupported")}</p>;
  }

  return (
    <div className="flex items-center gap-3">
      <audio ref={ref} src={src} preload="metadata" />
      <button
        type="button"
        aria-label={t(playing ? "calls.pauseRecording" : "calls.playRecording")}
        className="flex size-9 shrink-0 items-center justify-center rounded-full bg-accent text-accent-ink hover:opacity-90"
        onClick={() => {
          const el = ref.current;
          if (el === null) {
            return;
          }
          if (playing) {
            el.pause();
            setPlaying(false);
          } else {
            void el.play().then(() => setPlaying(true)).catch(() => setFailed(true));
          }
        }}
      >
        {playing ? "❚❚" : "▶"}
      </button>
      <input
        type="range"
        className="flex-1 accent-[var(--fd-accent)]"
        min={0}
        max={duration || 0}
        step={0.1}
        value={time}
        aria-label={t("calls.duration")}
        onChange={(e) => {
          const el = ref.current;
          const next = Number(e.target.value);
          if (el !== null) {
            el.currentTime = next;
          }
          setTime(next);
        }}
      />
      <span className="w-20 text-right text-xs tabular-nums text-muted">
        {clock(time)} / {clock(duration)}
      </span>
    </div>
  );
}
