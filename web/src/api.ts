export class APIError extends Error {
  constructor(readonly status: number, message: string) {
    super(message);
  }
}

export async function requestJSON<T>(path: string, options: RequestInit = {}): Promise<T> {
  let response: Response;
  const timeout = AbortSignal.timeout(15000);
  const signal = options.signal ? AbortSignal.any([options.signal, timeout]) : timeout;
  try {
    response = await fetch(path, { cache: "no-store", ...options, signal });
  } catch (error) {
    if (options.signal?.aborted) throw error;
    throw new Error("Check your connection and try again.");
  }
  const data = await response.json().catch(() => null);
  if (!response.ok) {
    throw new APIError(response.status, typeof data?.error === "string" ? data.error : "The server couldn’t complete the request. Try again.");
  }
  if (data === null) throw new Error("The server response was incomplete. Check your connection and try again.");
  return data;
}

export function postJSON<T>(path: string, body: object = {}): Promise<T> {
  return requestJSON<T>(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

export const botPersonalities = {
  noob: { name: "First-timer", description: "Looks for a pattern line it can fill this turn." },
  beginner: { name: "Beginner", description: "Tries to fit more tiles onto its lines and keep extras off the floor." },
  intermediate: { name: "Intermediate", description: "Chooses the draft that looks best for its own board." },
  competitive: { name: "Competitive", description: "Weighs each draft against your strongest immediate reply." },
  tactical: { name: "Tactical", description: "Looks past your reply to plan a follow-up draft." },
  advanced: { name: "Advanced", description: "Looks several drafts ahead for scoring chances and opportunities to block you." },
  master: { name: "Master", description: "Searches furthest ahead, playing out the rest of the round when it can." },
} as const;
export type BotLevel = keyof typeof botPersonalities;
export type Seat = { number: number; displayName: string; isBot: boolean; botLevel?: BotLevel };

export function seatName(seat: Seat) {
  return seat.isBot ? `${seat.displayName} (${botPersonalities[seat.botLevel ?? "competitive"].name})` : seat.displayName;
}

export type Room = {
  id: string;
  slug: string;
  listed: boolean;
  seats: Seat[];
  yourSeat: number;
  game: Game | null;
};

export type LobbyRoom = { slug: string; hostName: string; isHost: boolean };

export function roomPath(slug: string) {
  return `/room/${encodeURIComponent(slug)}`;
}

export function roomName(slug: string) {
  return slug.split("-").map(word => word.charAt(0).toUpperCase() + word.slice(1)).join(" ");
}

export function inviteURL(slug: string) {
  // A player on the old Railway host stays there to keep their guest cookie.
  // Invitations opened by a friend can use the public domain.
  const deployed = ["azulwithfriends.com", "www.azulwithfriends.com", "azulwithfriends-production.up.railway.app"];
  const origin = deployed.includes(window.location.hostname) ? "https://azulwithfriends.com" : window.location.origin;
  return new URL(roomPath(slug), origin).href;
}

export const colors = ["blue", "yellow", "red", "black", "white"] as const;
export type Color = typeof colors[number];
export type Tile = Color | "first";
export type Move = { source: number; color: Color; row: number };
export type Board = {
  score: number;
  lines: { color: Color | ""; count: number }[];
  wall: boolean[][];
  floor: Tile[];
  bonus: { rows: number; columns: number; colors: number };
};
export type RoundScore = { wall: number; penalty: number };
export type HistoricalTable = {
  round: number;
  turn: number;
  finished: boolean;
  factories: Color[][];
  center: Color[];
  markerInCenter: boolean;
  players: Board[];
  winners: number[];
  lastMove?: (Move & { seat: number; count: number }) | null;
  lastRound?: { round: number; players: RoundScore[] } | null;
};

export type Game = HistoricalTable & {
  matchId?: string;
  revision: number;
  anticipatedScores: [number, number];
  legalMoves: Move[];
};

export type HistoryEvent = {
  kind: "opening" | "baseline" | "draft" | "scoring" | "refill" | "final";
  table: HistoricalTable;
};

export type MatchHistory = {
  matchId: string;
  revision: number;
  complete: boolean;
  events: HistoryEvent[];
};
