import type { Tile } from "./api";

export function Ceramic({ color, ghost = false, preview = false, pending = false }: { color?: Tile; ghost?: boolean; preview?: boolean; pending?: boolean }) {
  return <span aria-hidden="true" className={`ceramic ${color ?? "blank"}${ghost ? " ghost" : ""}${preview ? " preview" : ""}${pending ? " pending" : ""}`}>
    {color && <svg focusable="false" viewBox="0 0 32 32"><use href={`#motif-${color}`}/></svg>}
  </span>;
}
