const key = "azul.last-game.v1";
const slugPattern = /^[a-z]{1,16}(?:-[a-z]{1,16}){3}$/;

export function rememberGame(slug: string) {
  try {
    localStorage.setItem(key, slug);
  } catch {
    // The lobby can still find an unfinished game through the guest session.
  }
}

export function recentGame(): string | null {
  try {
    const slug = localStorage.getItem(key);
    return slug && slugPattern.test(slug) ? slug : null;
  } catch {
    return null;
  }
}
