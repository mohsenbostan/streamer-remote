import { useEffect, useState } from "react"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { api, type Permission, type StatusResponse, type TwitchAuthState } from "@/lib/api"
import { usePolling } from "@/hooks/usePolling"
import { toast } from "sonner"
import { ExternalLink, Loader2 } from "lucide-react"

/** Shared by both the first-time setup flow and the reconnect flow: shows
 * the device code once one has been requested, otherwise nothing. */
function DeviceCodePrompt({ auth }: { auth: TwitchAuthState | null }) {
  if (auth?.state !== "pending" || !auth.verificationUri) return null
  return (
    <div className="flex flex-col items-center gap-3 rounded-lg border bg-secondary/40 py-8 text-center">
      <Loader2 className="size-5 animate-spin text-accent-brand" />
      <p className="text-sm text-muted-foreground">
        Open{" "}
        <a
          className="font-medium text-accent-brand underline underline-offset-2"
          href={auth.verificationUri}
          target="_blank"
          rel="noreferrer"
        >
          {auth.verificationUri}
        </a>{" "}
        and enter this code:
      </p>
      <p className="font-mono text-2xl font-semibold tracking-widest">{auth.userCode}</p>
    </div>
  )
}

// Shown only when Twitch has never been configured at all — asks for the
// channel name and Client ID once, then hands off to the same device-code
// flow the reconnect card uses.
function SetupCard({ onConnected }: { onConnected: () => void }) {
  const [channel, setChannel] = useState("")
  const [clientId, setClientId] = useState("")
  const [connecting, setConnecting] = useState(false)
  const { data: auth } = usePolling<TwitchAuthState>(
    () => api.twitchAuthState(),
    connecting ? 1500 : 60_000,
  )

  useEffect(() => {
    if (auth?.state === "connected" && connecting) {
      onConnected()
    }
  }, [auth?.state, connecting, onConnected])

  async function connect() {
    if (!channel.trim() || !clientId.trim()) {
      toast.error("Enter both your channel name and Client ID")
      return
    }
    try {
      await api.setupTwitch(channel.trim().toLowerCase(), clientId.trim())
      await api.connectTwitch()
      setConnecting(true)
    } catch (e) {
      toast.error("Setup failed", { description: String(e) })
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Connect to Twitch</CardTitle>
        <CardDescription>
          Register a free app at{" "}
          <a
            className="inline-flex items-center gap-1 text-accent-brand underline underline-offset-2"
            href="https://dev.twitch.tv/console/apps"
            target="_blank"
            rel="noreferrer"
          >
            dev.twitch.tv/console/apps <ExternalLink className="size-3" />
          </a>{" "}
          — Category "Chat Bot", Client Type "Public", Redirect URL "http://localhost".
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {auth?.state === "pending" ? (
          <DeviceCodePrompt auth={auth} />
        ) : (
          <>
            <div className="grid gap-2">
              <Label htmlFor="channel">Twitch channel</Label>
              <Input
                id="channel"
                placeholder="yourchannelname"
                value={channel}
                onChange={(e) => setChannel(e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="clientId">Client ID</Label>
              <Input
                id="clientId"
                placeholder="paste it here"
                value={clientId}
                onChange={(e) => setClientId(e.target.value)}
              />
            </div>
            {auth?.state === "error" && (
              <p className="text-sm text-destructive">{auth.error}</p>
            )}
            <Button onClick={connect} className="self-start">
              Save &amp; Connect
            </Button>
          </>
        )}
      </CardContent>
    </Card>
  )
}

// Shown when a channel/Client ID are already saved but the connection
// isn't up (app just started and hasn't finished, connection dropped, or
// the cached login expired). Never re-asks for channel/Client ID — those
// are already known — it just (re)runs the device-code login.
function ReconnectCard({ channel, onConnected }: { channel: string; onConnected: () => void }) {
  const [connecting, setConnecting] = useState(false)
  const { data: auth } = usePolling<TwitchAuthState>(
    () => api.twitchAuthState(),
    connecting ? 1500 : 60_000,
  )

  useEffect(() => {
    if (auth?.state === "connected" && connecting) {
      onConnected()
    }
  }, [auth?.state, connecting, onConnected])

  async function connect() {
    try {
      await api.connectTwitch()
      setConnecting(true)
    } catch (e) {
      toast.error("Couldn't connect", { description: String(e) })
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Reconnect to Twitch</CardTitle>
        <CardDescription>
          Set up for #{channel}, but not connected right now.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {auth?.state === "pending" ? (
          <DeviceCodePrompt auth={auth} />
        ) : (
          <>
            {auth?.state === "error" && <p className="text-sm text-destructive">{auth.error}</p>}
            <Button onClick={connect} disabled={connecting} className="self-start">
              Connect
            </Button>
          </>
        )}
      </CardContent>
    </Card>
  )
}

function QuickTestCard() {
  const [permission, setPermission] = useState<Permission>("broadcaster")
  const [text, setText] = useState("")
  const [sending, setSending] = useState(false)

  async function send() {
    if (!text.trim()) return
    setSending(true)
    try {
      await api.test(permission, text.trim())
      toast.success("Sent", { description: text })
      setText("")
    } catch (e) {
      toast.error("Couldn't send test command", { description: String(e) })
    } finally {
      setSending(false)
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Quick test</CardTitle>
        <CardDescription>
          Send a command as if it came from chat — handy for trying out binds without going live.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-wrap items-end gap-3">
        <div className="grid gap-2">
          <Label>As</Label>
          <Select value={permission} onValueChange={(v) => setPermission(v as Permission)}>
            <SelectTrigger className="w-40">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="everyone">Everyone</SelectItem>
              <SelectItem value="subscriber">Subscriber</SelectItem>
              <SelectItem value="vip">VIP</SelectItem>
              <SelectItem value="moderator">Moderator</SelectItem>
              <SelectItem value="broadcaster">Broadcaster</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="grid flex-1 gap-2">
          <Label>Command</Label>
          <Input
            placeholder="rc!w+shift"
            value={text}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && send()}
          />
        </div>
        <Button onClick={send} disabled={sending}>
          Send
        </Button>
      </CardContent>
    </Card>
  )
}

export function OverviewTab({
  status,
  onChanged,
}: {
  status: StatusResponse | null
  onChanged: () => void
}) {
  const showSetup = status && !status.localOnly && !status.twitchConfigured
  const showReconnect = status && !status.localOnly && status.twitchConfigured && !status.twitchConnected

  return (
    <div className="flex flex-col gap-6">
      {showSetup && <SetupCard onConnected={onChanged} />}
      {showReconnect && <ReconnectCard channel={status.channel} onConnected={onChanged} />}
      <QuickTestCard />
    </div>
  )
}
