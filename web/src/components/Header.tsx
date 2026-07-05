import type { ReactElement } from "react"
import { Switch } from "@/components/ui/switch"
import { Badge } from "@/components/ui/badge"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { Gamepad2 } from "lucide-react"
import type { StatusResponse } from "@/lib/api"
import { api } from "@/lib/api"
import { toast } from "sonner"

function Dot({ connected }: { connected: boolean }) {
  return <span className={`size-2 rounded-full ${connected ? "bg-success" : "bg-warning"}`} />
}

// One badge per configured platform, since a streamer can be live on
// Twitch and Kick at once — each connection is tracked and shown
// independently rather than collapsed into a single status.
function PlatformBadges({ status }: { status: StatusResponse }) {
  const badges: ReactElement[] = []
  if (status.twitchConfigured) {
    badges.push(
      <Badge key="twitch" variant="secondary" className="gap-1.5 font-normal">
        <Dot connected={status.twitchConnected} />
        Twitch — #{status.channel}
      </Badge>,
    )
  }
  if (status.kickConfigured) {
    badges.push(
      <Badge key="kick" variant="secondary" className="gap-1.5 font-normal">
        <Dot connected={status.kickConnected} />
        Kick — {status.kickChannel}
      </Badge>,
    )
  }
  if (badges.length === 0) {
    badges.push(
      <Badge key="none" variant="secondary" className="gap-1.5 font-normal">
        <span className="size-2 rounded-full bg-muted-foreground/40" />
        Not set up
      </Badge>,
    )
  }
  return <>{badges}</>
}

export function Header({
  status,
  onChanged,
}: {
  status: StatusResponse | null
  onChanged: () => void
}) {
  async function togglePause(active: boolean) {
    try {
      await (active ? api.resume() : api.pause())
      onChanged()
    } catch (e) {
      toast.error("Couldn't change remote state", { description: String(e) })
    }
  }

  return (
    <header className="flex items-center justify-between border-b px-6 py-4">
      <div className="flex items-center gap-2.5">
        <Gamepad2 className="size-5 text-accent-brand" />
        <span className="font-semibold tracking-tight">Streamer Remote</span>
        {status && status.localOnly && (
          <Badge variant="secondary" className="ml-1 gap-1.5 font-normal">
            <span className="size-2 rounded-full bg-accent-brand" />
            Local test mode
          </Badge>
        )}
        {status && !status.localOnly && (
          <div className="ml-1 flex items-center gap-2">
            <PlatformBadges status={status} />
          </div>
        )}
      </div>

      <div className="flex items-center gap-4">
        {status && (
          <Tooltip>
            <TooltipTrigger asChild>
              <div className="flex items-center gap-2">
                <span className="text-sm text-muted-foreground">
                  {status.paused ? "Paused" : "Active"}
                </span>
                <Switch checked={!status.paused} onCheckedChange={togglePause} />
              </div>
            </TooltipTrigger>
            <TooltipContent>
              {status.paused
                ? "Remote is paused — no commands will run"
                : "Remote is active — chat and rewards can trigger input"}
            </TooltipContent>
          </Tooltip>
        )}
        {status && <span className="text-xs text-muted-foreground">{status.version}</span>}
      </div>
    </header>
  )
}
