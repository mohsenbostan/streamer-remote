import { useEffect, useRef, useState } from "react"

/**
 * Polls fn on an interval and exposes the latest result. Errors are
 * swallowed after the first successful load (transient network hiccups
 * shouldn't blank out a working dashboard); refresh() re-runs immediately.
 */
export function usePolling<T>(fn: () => Promise<T>, intervalMs: number) {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState<string | null>(null)
  const fnRef = useRef(fn)
  fnRef.current = fn

  const refresh = () => {
    fnRef.current()
      .then((d) => {
        setData(d)
        setError(null)
      })
      .catch((e) => setError(String(e)))
  }

  useEffect(() => {
    refresh()
    const id = setInterval(refresh, intervalMs)
    return () => clearInterval(id)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [intervalMs])

  return { data, error, refresh }
}
