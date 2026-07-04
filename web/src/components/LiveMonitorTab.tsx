import { useEffect, useRef, useState } from "react"
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { ScrollArea } from "@/components/ui/scroll-area"
import { subscribeEvents, type LiveEvent } from "@/lib/api"
import {
  AlertTriangle,
  Gamepad2,
  Gift,
  Pause,
  Play,
  ShieldOff,
  Wifi,
  WifiOff,
  Info,
} from "lucide-react"

const MAX_EVENTS = 300

function iconFor(msg: string) {
  const m = msg.toLowerCase()
  if (m.includes("redeemed") || m.includes("redemption")) return Gift
  if (m.includes("blocked")) return ShieldOff
  if (m.includes("paused")) return Pause
  if (m.includes("resumed")) return Play
  if (m.includes("connected to twitch") || m.includes("authorization complete")) return Wifi
  if (m.includes("lost") || m.includes("reconnecting") || m.includes("failed")) return WifiOff
  if (m.includes("dispatched")) return Gamepad2
  if (m.includes("error")) return AlertTriangle
  return Info
}

function levelClasses(level: LiveEvent["level"]) {
  switch (level) {
    case "ERROR":
      return "text-destructive"
    case "WARN":
      return "text-warning"
    case "DEBUG":
      return "text-muted-foreground"
    default:
      return "text-foreground"
  }
}

function formatTime(iso: string) {
  try {
    return new Date(iso).toLocaleTimeString()
  } catch {
    return iso
  }
}

export function LiveMonitorTab() {
  const [events, setEvents] = useState<LiveEvent[]>([])
  const [paused, setPaused] = useState(false)
  const pausedRef = useRef(paused)
  pausedRef.current = paused

  useEffect(() => {
    return subscribeEvents((e) => {
      if (pausedRef.current) return
      setEvents((prev) => [e, ...prev].slice(0, MAX_EVENTS))
    })
  }, [])

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <div>
          <CardTitle>Live monitor</CardTitle>
          <CardDescription>Every command, block, and connection event as it happens.</CardDescription>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={() => setPaused((p) => !p)}>
            {paused ? "Resume feed" : "Freeze feed"}
          </Button>
          <Button variant="outline" size="sm" onClick={() => setEvents([])}>
            Clear
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        <ScrollArea className="h-[60vh] rounded-md border">
          {events.length === 0 ? (
            <p className="p-6 text-center text-sm text-muted-foreground">
              Nothing yet — events will appear here in real time.
            </p>
          ) : (
            <div className="divide-y">
              {events.map((e, i) => {
                const Icon = iconFor(e.msg)
                return (
                  <div key={i} className="flex items-start gap-3 px-4 py-2.5 text-sm">
                    <Icon className={`mt-0.5 size-4 shrink-0 ${levelClasses(e.level)}`} />
                    <div className="min-w-0 flex-1">
                      <div className="flex items-baseline gap-2">
                        <span className={levelClasses(e.level)}>{e.msg}</span>
                        <span className="shrink-0 text-xs text-muted-foreground">
                          {formatTime(e.time)}
                        </span>
                      </div>
                      {Object.keys(e.attrs).length > 0 && (
                        <p className="truncate text-xs text-muted-foreground">
                          {Object.entries(e.attrs)
                            .map(([k, v]) => `${k}=${v}`)
                            .join("  ")}
                        </p>
                      )}
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </ScrollArea>
      </CardContent>
    </Card>
  )
}
