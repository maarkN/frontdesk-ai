/** Translated labels and badge tones for domain enums. */
import type { TFunction } from "i18next";

import type { CallStatus, CallSummary, EndReason, Urgency } from "@frontdesk/shared";

import type { BadgeTone } from "./components/ui";

/** Human label for a call's lifecycle: end reason once ended, else status. */
export function callStateLabel(
  t: TFunction,
  status: CallStatus,
  endReason: EndReason | undefined,
): string {
  if (endReason !== undefined) {
    return t(`calls.endReason.${endReason}`);
  }
  if (status === "") {
    return t("calls.status.unknown");
  }
  return t(`calls.status.${status}`);
}

export function callStateTone(summary: Pick<CallSummary, "status" | "endReason">): BadgeTone {
  if (summary.endReason === "error") {
    return "danger";
  }
  if (summary.endReason === "voicemail") {
    return "warning";
  }
  if (summary.endReason === "transferred" || summary.status === "transferred") {
    return "accent";
  }
  if (summary.status === "answered" || summary.status === "started") {
    return "success"; // Live right now.
  }
  return "neutral";
}

export function urgencyLabel(t: TFunction, urgency: Urgency): string {
  return t(`messages.urgencyLevel.${urgency}`);
}

const URGENCY_TONES: Record<Urgency, BadgeTone> = {
  emergency: "danger",
  high: "warning",
  normal: "accent",
  low: "neutral",
};

export function urgencyTone(urgency: Urgency): BadgeTone {
  return URGENCY_TONES[urgency];
}
