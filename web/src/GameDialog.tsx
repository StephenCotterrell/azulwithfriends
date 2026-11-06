import { useEffect, useId, useRef, type ReactNode } from "react";

export function GameDialog({ title, onClose, children, className = "" }: {
  title: string; onClose: () => void; children: ReactNode; className?: string;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const heading = useId();

  useEffect(() => {
    if (!dialog.current?.open) dialog.current?.showModal();
  }, []);

  return <dialog ref={dialog} className={`game-dialog ${className}`.trim()} aria-labelledby={heading} onClose={onClose}>
    <header className="game-dialog-header">
      <h2 id={heading}>{title}</h2>
      <button type="button" className="secondary" autoFocus onClick={() => dialog.current?.close()}>Close</button>
    </header>
    <div className="game-dialog-body">{children}</div>
  </dialog>;
}
