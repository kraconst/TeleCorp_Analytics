import { useEffect, useState } from 'react';

export function useNow(intervalMs = 30_000) {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const tick = () => setNow(new Date());
    document.addEventListener('visibilitychange', tick);
    const timer = window.setInterval(tick, intervalMs);
    return () => {
      document.removeEventListener('visibilitychange', tick);
      window.clearInterval(timer);
    };
  }, [intervalMs]);
  return now;
}
