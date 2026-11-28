import { botPersonalities, type BotLevel } from "./api";

export type SoloAttempt = { id: string; level: BotLevel };
const attemptKey = "azul.solo-attempt.v1";

export function savedAttempt(fallback: SoloAttempt | null = null): SoloAttempt | null {
  try {
    const value = JSON.parse(sessionStorage.getItem(attemptKey) ?? "null");
    if (value && typeof value.id === "string" && /^[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$/i.test(value.id)
      && typeof value.level === "string" && Object.hasOwn(botPersonalities, value.level)) {
      return { id: value.id, level: value.level };
    }
  } catch { return fallback; }
  return null;
}

export function storeAttempt(attempt: SoloAttempt | null) {
  try {
    if (attempt) sessionStorage.setItem(attemptKey, JSON.stringify(attempt));
    else sessionStorage.removeItem(attemptKey);
    return true;
  } catch { return false; }
}

export function forgetRecoveredAttempt(roomId: string) {
  if (savedAttempt()?.id === roomId) storeAttempt(null);
}
