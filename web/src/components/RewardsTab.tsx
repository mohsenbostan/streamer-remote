import { useState } from "react"
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Badge } from "@/components/ui/badge"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { api, type RewardAction, type RewardProfilesResponse } from "@/lib/api"
import { usePolling } from "@/hooks/usePolling"
import { toast } from "sonner"
import { Gift, Plus, Trash2, Layers, Check } from "lucide-react"

function RewardProfilesCard({ onActivated }: { onActivated: () => void }) {
  const { data, refresh } = usePolling<RewardProfilesResponse>(() => api.rewardProfiles(), 15_000)
  const [saveOpen, setSaveOpen] = useState(false)
  const [name, setName] = useState("")
  const [busy, setBusy] = useState<string | null>(null)

  const profiles = data?.profiles ?? []
  const active = data?.active ?? ""

  async function saveCurrent() {
    if (!name.trim()) {
      toast.error("Give the profile a name")
      return
    }
    setBusy("save")
    try {
      await api.saveRewardProfile(name.trim())
      toast.success(`Saved current rewards as "${name.trim()}"`)
      setSaveOpen(false)
      setName("")
      refresh()
    } catch (e) {
      toast.error("Couldn't save profile", { description: String(e) })
    } finally {
      setBusy(null)
    }
  }

  async function activate(profileName: string) {
    setBusy(profileName)
    try {
      await api.activateRewardProfile(profileName)
      toast.success(`Switched to "${profileName}"`)
      refresh()
      onActivated()
    } catch (e) {
      toast.error("Couldn't switch profile", { description: String(e) })
    } finally {
      setBusy(null)
    }
  }

  async function remove(profileName: string) {
    try {
      await api.deleteRewardProfile(profileName)
      toast.success(`Deleted "${profileName}"`)
      refresh()
    } catch (e) {
      toast.error("Couldn't delete profile", { description: String(e) })
    }
  }

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <div>
          <CardTitle>Reward profiles</CardTitle>
          <CardDescription>
            Save the rewards below as a named profile, then switch between profiles to swap out
            what's live on Twitch in one go.
          </CardDescription>
        </div>
        <Dialog open={saveOpen} onOpenChange={setSaveOpen}>
          <DialogTrigger asChild>
            <Button size="sm" variant="outline" className="gap-1.5">
              <Plus className="size-4" /> Save current as profile
            </Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Save current rewards as a profile</DialogTitle>
              <DialogDescription>
                Snapshots the channel-points-only actions below under this name.
              </DialogDescription>
            </DialogHeader>
            <div className="grid gap-2 py-2">
              <Label>Profile name</Label>
              <Input placeholder="Speedrun night" value={name} onChange={(e) => setName(e.target.value)} />
            </div>
            <DialogFooter>
              <Button onClick={saveCurrent} disabled={busy === "save"}>
                Save
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </CardHeader>
      <CardContent>
        {profiles.length === 0 ? (
          <div className="flex flex-col items-center gap-2 rounded-lg border border-dashed py-10 text-center text-sm text-muted-foreground">
            <Layers className="size-6" />
            No saved profiles yet.
          </div>
        ) : (
          <div className="flex flex-col gap-2">
            {profiles.map((p) => (
              <div
                key={p.name}
                className="flex items-center justify-between rounded-lg border px-4 py-3"
              >
                <div className="flex items-center gap-3">
                  {p.name === active ? (
                    <Badge className="gap-1">
                      <Check className="size-3" /> Active
                    </Badge>
                  ) : (
                    <Badge variant="secondary">{p.rewards.length} rewards</Badge>
                  )}
                  <span className="text-sm">{p.name}</span>
                </div>
                <div className="flex items-center gap-2">
                  <Button
                    size="sm"
                    variant="secondary"
                    disabled={p.name === active || busy !== null}
                    onClick={() => activate(p.name)}
                  >
                    {busy === p.name ? "Switching…" : "Activate"}
                  </Button>
                  <Button variant="ghost" size="icon" onClick={() => remove(p.name)}>
                    <Trash2 className="size-4 text-muted-foreground" />
                  </Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

export function RewardsTab() {
  const { data: rewards, refresh } = usePolling<RewardAction[]>(() => api.rewards(), 15_000)
  const [open, setOpen] = useState(false)
  const [action, setAction] = useState("")
  const [title, setTitle] = useState("")
  const [cost, setCost] = useState("500")
  const [saving, setSaving] = useState(false)

  async function add() {
    const costNum = Number(cost)
    if (!action.trim() || !title.trim() || !Number.isInteger(costNum) || costNum <= 0) {
      toast.error("Fill in an action, title, and a positive point cost")
      return
    }
    setSaving(true)
    try {
      await api.addReward(action.trim().toLowerCase(), title.trim(), costNum)
      toast.success(`Created "${title}" on Twitch`)
      setOpen(false)
      setAction("")
      setTitle("")
      setCost("500")
      refresh()
    } catch (e) {
      toast.error("Twitch rejected the reward", { description: String(e) })
    } finally {
      setSaving(false)
    }
  }

  async function remove(r: RewardAction) {
    try {
      await api.removeReward(r.rewardId)
      toast.success(`Removed "${r.rewardTitle}"`)
      refresh()
    } catch (e) {
      toast.error("Couldn't remove reward", { description: String(e) })
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <RewardProfilesCard onActivated={refresh} />
      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <div>
            <CardTitle>Channel-points-only actions</CardTitle>
            <CardDescription>
              These only run when redeemed on Twitch — typing them in chat is blocked for viewers
              (mods are exempt).
            </CardDescription>
          </div>
          <Dialog open={open} onOpenChange={setOpen}>
            <DialogTrigger asChild>
              <Button size="sm" className="gap-1.5">
                <Plus className="size-4" /> Add
              </Button>
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>New channel-points-only action</DialogTitle>
                <DialogDescription>
                  This creates a real reward on your Twitch channel automatically.
                </DialogDescription>
              </DialogHeader>
              <div className="flex flex-col gap-4 py-2">
                <div className="grid gap-2">
                  <Label>Action</Label>
                  <Input
                    placeholder="alt+f4"
                    value={action}
                    onChange={(e) => setAction(e.target.value)}
                  />
                  <p className="text-xs text-muted-foreground">
                    Same syntax as a chat command, without the prefix — e.g. <code>lwin</code>,{" "}
                    <code>move:50:-30</code>, or a sequence like{" "}
                    <code>alt+f10,wait:800,enter</code>.
                  </p>
                </div>
                <div className="grid gap-2">
                  <Label>Reward title</Label>
                  <Input
                    placeholder="Rage Quit"
                    maxLength={45}
                    value={title}
                    onChange={(e) => setTitle(e.target.value)}
                  />
                </div>
                <div className="grid gap-2">
                  <Label>Point cost</Label>
                  <Input
                    type="number"
                    min={1}
                    value={cost}
                    onChange={(e) => setCost(e.target.value)}
                  />
                </div>
              </div>
              <DialogFooter>
                <Button onClick={add} disabled={saving}>
                  Create on Twitch
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        </CardHeader>
        <CardContent>
          {!rewards || rewards.length === 0 ? (
            <div className="flex flex-col items-center gap-2 rounded-lg border border-dashed py-10 text-center text-sm text-muted-foreground">
              <Gift className="size-6" />
              No channel-points-only actions yet.
            </div>
          ) : (
            <div className="flex flex-col gap-2">
              {rewards.map((r) => (
                <div
                  key={r.rewardId}
                  className="flex items-center justify-between rounded-lg border px-4 py-3"
                >
                  <div className="flex items-center gap-3">
                    <Badge variant="secondary" className="font-mono">
                      {r.action}
                    </Badge>
                    <span className="text-sm">{r.rewardTitle}</span>
                    <span className="text-xs text-muted-foreground">{r.cost} pts</span>
                  </div>
                  <Button variant="ghost" size="icon" onClick={() => remove(r)}>
                    <Trash2 className="size-4 text-muted-foreground" />
                  </Button>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
