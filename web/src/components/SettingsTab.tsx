import { useEffect, useState } from "react"
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { Separator } from "@/components/ui/separator"
import { api, type Settings, type StatusResponse, type UpdateInfo } from "@/lib/api"
import { usePolling } from "@/hooks/usePolling"
import { toast } from "sonner"
import { DownloadCloud, LogOut, RefreshCw } from "lucide-react"

function Field({
  label,
  hint,
  children,
}: {
  label: string
  hint?: string
  children: React.ReactNode
}) {
  return (
    <div className="grid gap-1.5">
      <Label>{label}</Label>
      {children}
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  )
}

function blacklistToText(combos: string[][]) {
  return combos.map((c) => c.join("+")).join("\n")
}
function textToBlacklist(text: string): string[][] {
  return text
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => line.split("+").map((k) => k.trim().toLowerCase()).filter(Boolean))
}

export function SettingsTab({
  status,
  onChanged,
}: {
  status: StatusResponse | null
  onChanged: () => void
}) {
  const { data: loaded, refresh } = usePolling<Settings>(() => api.settings(), 60_000)
  const [settings, setSettings] = useState<Settings | null>(null)
  const [deniedKeysText, setDeniedKeysText] = useState("")
  const [deniedCombosText, setDeniedCombosText] = useState("")
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (loaded) {
      setSettings(loaded)
      setDeniedKeysText(loaded.blacklist.deniedKeys.join(", "))
      setDeniedCombosText(blacklistToText(loaded.blacklist.deniedCombos))
    }
  }, [loaded])

  async function save() {
    if (!settings) return
    setSaving(true)
    try {
      const next: Settings = {
        ...settings,
        blacklist: {
          deniedKeys: deniedKeysText
            .split(",")
            .map((k) => k.trim().toLowerCase())
            .filter(Boolean),
          deniedCombos: textToBlacklist(deniedCombosText),
        },
      }
      await api.updateSettings(next)
      toast.success("Settings saved")
      refresh()
    } catch (e) {
      toast.error("Couldn't save settings", { description: String(e) })
    } finally {
      setSaving(false)
    }
  }

  if (!settings) return null

  const set = <K extends keyof Settings>(key: K, value: Settings[K]) =>
    setSettings({ ...settings, [key]: value })

  return (
    <div className="flex flex-col gap-6">
      <Card>
        <CardHeader>
          <CardTitle>Behavior</CardTitle>
          <CardDescription>How chat commands are parsed and rate-limited.</CardDescription>
        </CardHeader>
        <CardContent className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field label="Command prefix" hint="e.g. rc! — kept unusual to avoid colliding with other bots">
            <Input value={settings.prefix} onChange={(e) => set("prefix", e.target.value)} />
          </Field>
          <Field label="Max combo size" hint="Keys chained with '+' in one step, e.g. ctrl+shift+w">
            <Input
              type="number"
              min={1}
              max={20}
              value={settings.maxComboSize}
              onChange={(e) => set("maxComboSize", Number(e.target.value))}
            />
          </Field>
          <Field label="Max sequence steps" hint="Comma-separated steps in one command, e.g. alt+f10,wait:800,enter">
            <Input
              type="number"
              min={1}
              max={20}
              value={settings.maxSequenceSteps}
              onChange={(e) => set("maxSequenceSteps", Number(e.target.value))}
            />
          </Field>
          <Field label="Global cooldown (ms)" hint="Minimum time between any two commands">
            <Input
              type="number"
              min={0}
              value={settings.globalCooldownMs}
              onChange={(e) => set("globalCooldownMs", Number(e.target.value))}
            />
          </Field>
          <Field label="Per-viewer cooldown (ms)">
            <Input
              type="number"
              min={0}
              value={settings.perUserCooldownMs}
              onChange={(e) => set("perUserCooldownMs", Number(e.target.value))}
            />
          </Field>
          <Field label="Tap hold (ms)" hint="How long a tapped key is held down">
            <Input
              type="number"
              min={1}
              value={settings.tapHoldMs}
              onChange={(e) => set("tapHoldMs", Number(e.target.value))}
            />
          </Field>
          <Field label="Max hold (ms)" hint="Upper bound for an explicit hold: or wait: duration">
            <Input
              type="number"
              min={1}
              value={settings.maxHoldMs}
              onChange={(e) => set("maxHoldMs", Number(e.target.value))}
            />
          </Field>
          <Field label="Max mouse move (px)" hint="Upper bound per axis, including move:<dx>:<dy>">
            <Input
              type="number"
              min={1}
              value={settings.maxMoveStep}
              onChange={(e) => set("maxMoveStep", Number(e.target.value))}
            />
          </Field>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Access</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="flex items-center justify-between rounded-lg border px-4 py-3">
            <div>
              <p className="text-sm font-medium">Moderator-only mode</p>
              <p className="text-xs text-muted-foreground">
                Only moderators and the broadcaster can trigger commands
              </p>
            </div>
            <Switch
              checked={settings.modOnlyMode}
              onCheckedChange={(v) => set("modOnlyMode", v)}
            />
          </div>
          <div className="flex items-center justify-between rounded-lg border px-4 py-3">
            <div>
              <p className="text-sm font-medium">Text to speech</p>
              <p className="text-xs text-muted-foreground">
                Speak chat messages that start with rc-say:
              </p>
            </div>
            <Switch
              checked={settings.textToSpeechEnabled}
              onCheckedChange={(v) => set("textToSpeechEnabled", v)}
            />
          </div>
          <div className="flex items-center justify-between rounded-lg border px-4 py-3">
            <div>
              <p className="text-sm font-medium">Verbose logging</p>
              <p className="text-xs text-muted-foreground">
                Show every rejected/dropped command in the live monitor and log file
              </p>
            </div>
            <Switch checked={settings.logDebug} onCheckedChange={(v) => set("logDebug", v)} />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Blacklist</CardTitle>
          <CardDescription>
            Nothing is blocked by default. Add keys or combos here to block them for everyone
            except mods and the broadcaster.
          </CardDescription>
        </CardHeader>
        <CardContent className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field label="Denied keys" hint="Comma-separated, e.g. lwin, rwin">
            <Input value={deniedKeysText} onChange={(e) => setDeniedKeysText(e.target.value)} />
          </Field>
          <Field label="Denied combos" hint="One per line, e.g. alt+f4">
            <textarea
              className="min-h-20 rounded-md border bg-transparent px-3 py-2 text-sm shadow-xs outline-none focus-visible:ring-2 focus-visible:ring-ring"
              value={deniedCombosText}
              onChange={(e) => setDeniedCombosText(e.target.value)}
            />
          </Field>
        </CardContent>
      </Card>

      <div className="flex justify-end">
        <Button onClick={save} disabled={saving}>
          Save settings
        </Button>
      </div>

      <Separator />
      <TwitchAccountCard status={status} onChanged={onChanged} />

      <Separator />
      <UpdateCard />
    </div>
  )
}

function TwitchAccountCard({
  status,
  onChanged,
}: {
  status: StatusResponse | null
  onChanged: () => void
}) {
  const [loggingOut, setLoggingOut] = useState(false)

  if (!status || status.localOnly || !status.twitchConfigured) return null

  async function logout() {
    setLoggingOut(true)
    try {
      await api.logoutTwitch()
      toast.success("Disconnected from Twitch")
      onChanged()
    } catch (e) {
      toast.error("Couldn't disconnect", { description: String(e) })
    } finally {
      setLoggingOut(false)
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Twitch account</CardTitle>
        <CardDescription>Connected as #{status.channel}.</CardDescription>
      </CardHeader>
      <CardContent>
        <Button variant="outline" onClick={logout} disabled={loggingOut} className="gap-1.5">
          <LogOut className="size-4" /> Log out
        </Button>
        <p className="mt-2 text-xs text-muted-foreground">
          Forgets the saved channel, Client ID, and login — use this to set up a different Twitch
          account or channel from scratch.
        </p>
      </CardContent>
    </Card>
  )
}

function UpdateCard() {
  const { data: info, refresh } = usePolling<UpdateInfo>(() => api.checkUpdate(), 300_000)
  const [applying, setApplying] = useState(false)

  async function apply() {
    setApplying(true)
    try {
      await api.applyUpdate()
      toast.success("Updating and restarting…")
    } catch (e) {
      toast.error("Update failed", { description: String(e) })
      setApplying(false)
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Updates</CardTitle>
        <CardDescription>
          Current version: {info?.current ?? "…"}
          {info?.available && <> — {info.latest} is available</>}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex gap-2">
        <Button variant="outline" size="sm" onClick={refresh} className="gap-1.5">
          <RefreshCw className="size-4" /> Check for updates
        </Button>
        {info?.available && (
          <Button size="sm" onClick={apply} disabled={applying} className="gap-1.5">
            <DownloadCloud className="size-4" /> Update to {info.latest}
          </Button>
        )}
      </CardContent>
    </Card>
  )
}
