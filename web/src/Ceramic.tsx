import type { Tile } from "./api";
import "./ceramic.css";

export function Ceramic({ color, ghost = false, preview = false, pending = false }: { color?: Tile; ghost?: boolean; preview?: boolean; pending?: boolean }) {
  return <span aria-hidden="true" className={`ceramic ${color ?? "blank"}${ghost ? " ghost" : ""}${preview ? " preview" : ""}${pending ? " pending" : ""}`}>
    {color && <svg focusable="false" viewBox="0 0 32 32"><use href={`#motif-${color}`}/></svg>}
  </span>;
}

export function TilePatterns() {
  return <svg className="tile-patterns" aria-hidden="true" focusable="false">
    <defs>
      <symbol id="motif-blue" viewBox="0 0 32 32"><path fill="currentColor" d="M16 15C8 15 4 11 6 7s8-2 10 6c2-8 8-10 10-6s-2 8-10 8Zm0 2c8 0 12 4 10 8s-8 2-10-6c-2 8-8 10-10 6s2-8 10-8Z"/><circle cx="16" cy="16" r="2.3" fill="currentColor"/></symbol>
      <symbol id="motif-yellow" viewBox="0 0 32 32"><path fill="currentColor" d="m16 3 2.7 7.2 6.8-3.8-3.8 6.8L29 16l-7.3 2.8 3.8 6.8-6.8-3.8L16 29l-2.7-7.2-6.8 3.8 3.8-6.8L3 16l7.3-2.8-3.8-6.8 6.8 3.8Z"/></symbol>
      <symbol id="motif-red" viewBox="0 0 32 32"><path fill="none" stroke="currentColor" strokeWidth="2.2" d="m16 4 12 12-12 12L4 16Z"/><path fill="currentColor" d="m16 10 6 6-6 6-6-6Z"/></symbol>
      <symbol id="motif-black" viewBox="0 0 32 32"><path fill="none" stroke="currentColor" strokeWidth="2.5" d="m5 13 8-8M5 22 17 10m-7 17 12-12m-3 12 8-8M17 5l10 10M8 14l10 10"/></symbol>
      <symbol id="motif-white" viewBox="0 0 32 32"><g fill="none" stroke="currentColor" strokeWidth="2"><circle cx="16" cy="16" r="10"/><circle cx="16" cy="16" r="5"/><path d="M16 2v5m14 9h-5m-9 14v-5M2 16h5"/></g></symbol>
      <symbol id="motif-first" viewBox="0 0 32 32">
        <path d="M4 10V4h6m12 0h6v6m0 12v6h-6m-12 0H4v-6" fill="none" stroke="currentColor" strokeWidth="1.5"/>
        <text x="16" y="24" textAnchor="middle" fill="currentColor" fontFamily="Georgia, serif" fontSize="24" fontWeight="700">1</text>
      </symbol>
    </defs>
  </svg>;
}
