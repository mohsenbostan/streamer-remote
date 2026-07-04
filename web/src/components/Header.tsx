import { Switch } from "@/components/ui/switch"
import { Badge } from "@/components/ui/badge"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { Gamepad2 } from "lucide-react"
import type { StatusResponse } from "@/lib/api"
import { api } from "@/lib/api"
import { toast } from "sonner"

function ConnectionDot({ status }: { status: StatusResponse | null }) {
  if (!status) {
    return <span className="size-2 rounded-full bg-muted-foreground/40" />
  }
  if (status.localOnly) {
    return <span className="size-2 rounded-full bg-accent-brand" />
  }
  if (status.twitchConnected) {
    return <span className="size-2 rounded-full bg-success" />
  }
  return <span className="size-2 rounded-full bg-warning" />
}

function connectionLabel(status: StatusResponse | null): string {
  if (!status) return "Loading…"
  if (status.localOnly) return "Local test mode"
  if (!status.twitchConfigured) return "Not set up"
  if (status.twitchConnected) return `Connected — #${status.channel}`
  return "Connecting…"
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
        {status && (
          <Badge variant="secondary" className="ml-1 gap-1.5 font-normal">
            <ConnectionDot status={status} />
            {connectionLabel(status)}
          </Badge>
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
