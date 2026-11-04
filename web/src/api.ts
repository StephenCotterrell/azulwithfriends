export class APIError extends Error {
  constructor(readonly status: number, message: string) {
    super(message);
  }
}

export async function requestJSON<T>(path: string, options: RequestInit = {}): Promise<T> {
  let response: Response;
  try {
    response = await fetch(path, { cache: "no-store", ...options });
  } catch (error) {
    if (options.signal?.aborted) throw error;
    throw new Error("Check your connection and try again.");
  }
  const data = await response.json().catch(() => null);
  if (!response.ok || data === null) {
    throw new APIError(response.status, typeof data?.error === "string" ? data.error : "The server couldn’t complete the request. Try again.");
  }
  return data;
}

export function postJSON<T>(path: string, body: object = {}): Promise<T> {
  return requestJSON<T>(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

export type Room = {
  id: string;
  seats: { number: number; displayName: string }[];
  yourSeat: number;
};
