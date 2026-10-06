'use client'

import { useEffect, useState } from "react";

const REDIRECT_SECONDS = 5;

/** Counts down, then sends the visitor to the portal docs. The visitor can cancel and pick a link instead. */
const DocsRedirect = ({ href }: { href: string }) => {
  const [secondsLeft, setSecondsLeft] = useState(REDIRECT_SECONDS);
  const [cancelled, setCancelled] = useState(false);

  useEffect(() => {
    if (cancelled) {
      return;
    }
    if (secondsLeft <= 0) {
      window.location.assign(href);
      return;
    }
    const timer = window.setTimeout(() => setSecondsLeft((seconds) => seconds - 1), 1000);
    return () => window.clearTimeout(timer);
  }, [cancelled, secondsLeft, href]);

  if (cancelled) {
    return <p className="text-sm text-muted-foreground">Redirect cancelled. Pick a destination below.</p>;
  }

  return (
    <p className="text-sm text-muted-foreground" aria-live="polite">
      Taking you to the documentation in {secondsLeft}s.{" "}
      <button type="button" onClick={() => setCancelled(true)} className="text-cyan hover:underline">
        Stay here
      </button>
    </p>
  );
};

export default DocsRedirect;
