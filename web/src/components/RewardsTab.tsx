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
import {
  api,
  type RewardAction,
  type RewardDraft,
  type RewardProfile,
  type RewardProfilesResponse,
} from "@/lib/api"
import { usePolling } from "@/hooks/usePolling"
import { toast } from "sonner"
import { Gift, Plus, Trash2, Layers, Check, X, FolderPlus, Pencil } from "lucide-react"

const PROFILE_COLORS = [
  "#f87171",
  "#fb923c",
  "#facc15",
  "#4ade80",
  "#22d3ee",
  "#60a5fa",
  "#a78bfa",
  "#f472b6",
]

function ColorPicker({ value, onChange }: { value: string; onChange: (color: string) => void }) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <button
        type="button"
        aria-label="No color"
        onClick={() => onChange("")}
        className="flex size-6 items-center justify-center rounded-full border border-dashed text-muted-foreground"
      >
        {value === "" && <Check className="size-3.5" />}
      </button>
      {PROFILE_COLORS.map((c) => (
        <button
          key={c}
          type="button"
          aria-label={`Color ${c}`}
          onClick={() => onChange(c)}
          className="flex size-6 items-center justify-center rounded-full ring-offset-2 ring-offset-background"
          style={{ backgroundColor: c, boxShadow: value === c ? `0 0 0 2px ${c}` : undefined }}
        >
          {value === c && <Check className="size-3.5 text-black/70" />}
        </button>
      ))}
    </div>
  )
}

function DraftRewardEditor({
  drafts,
  setDrafts,
}: {
  drafts: RewardDraft[]
  setDrafts: (d: RewardDraft[]) => void
}) {
  const [action, setAction] = useState("")
  const [title, setTitle] = useState("")
  const [cost, setCost] = useState("500")

  function addRow() {
    const costNum = Number(cost)
    if (!action.trim() || !title.trim() || !Number.isInteger(costNum) || costNum <= 0) {
      toast.error("Fill in an action, title, and a positive point cost")
      return
    }
    setDrafts([...drafts, { action: action.trim().toLowerCase(), rewardTitle: title.trim(), cost: costNum }])
    setAction("")
    setTitle("")
    setCost("500")
  }

  return (
    <div className="grid gap-3">
      <div className="grid grid-cols-[1fr_1fr_100px_auto] items-end gap-2">
        <div className="grid gap-1.5">
          <Label className="text-xs">Action</Label>
          <Input placeholder="alt+f4" value={action} onChange={(e) => setAction(e.target.value)} />
        </div>
        <div className="grid gap-1.5">
          <Label className="text-xs">Reward title</Label>
          <Input placeholder="Rage Quit" maxLength={45} value={title} onChange={(e) => setTitle(e.target.value)} />
        </div>
        <div className="grid gap-1.5">
          <Label className="text-xs">Cost</Label>
          <Input type="number" min={1} value={cost} onChange={(e) => setCost(e.target.value)} />
        </div>
        <Button type="button" size="icon" variant="secondary" onClick={addRow}>
          <Plus className="size-4" />
        </Button>
      </div>
      {drafts.length > 0 && (
        <div className="flex flex-col gap-1.5">
          {drafts.map((d, i) => (
            <div key={i} className="flex items-center justify-between rounded-md border px-3 py-1.5">
              <div className="flex items-center gap-3">
                <Badge variant="secondary" className="font-mono">
                  {d.action}
                </Badge>
                <span className="text-sm">{d.rewardTitle}</span>
                <span className="text-xs text-muted-foreground">{d.cost} pts</span>
              </div>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                onClick={() => setDrafts(drafts.filter((_, j) => j !== i))}
              >
                <X className="size-4 text-muted-foreground" />
              </Button>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

function RewardProfilesCard({
  profiles,
  active,
  refresh,
  onActivated,
}: {
  profiles: RewardProfile[]
  active: string
  refresh: () => void
  onActivated: () => void
}) {
  const [newOpen, setNewOpen] = useState(false)
  const [newName, setNewName] = useState("")
  const [newColor, setNewColor] = useState("")
  const [newDrafts, setNewDrafts] = useState<RewardDraft[]>([])
  const [busy, setBusy] = useState<string | null>(null)
  const [editingProfile, setEditingProfile] = useState<RewardProfile | null>(null)
  const [editName, setEditName] = useState("")
  const [editColor, setEditColor] = useState("")
  const [editDrafts, setEditDrafts] = useState<RewardDraft[]>([])

  async function createNew() {
    if (!newName.trim()) {
      toast.error("Give the profile a name")
      return
    }
    if (newDrafts.length === 0) {
      toast.error("Add at least one reward to the profile")
      return
    }
    setBusy("new")
    try {
      await api.saveRewardProfile(newName.trim(), newColor, newDrafts)
      toast.success(`Created profile "${newName.trim()}"`)
      setNewOpen(false)
      setNewName("")
      setNewColor("")
      setNewDrafts([])
      refresh()
    } catch (e) {
      toast.error("Couldn't create profile", { description: String(e) })
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

  function openEdit(p: RewardProfile) {
    setEditingProfile(p)
    setEditName(p.name)
    setEditColor(p.color ?? "")
    setEditDrafts(p.rewards.map((r) => ({ action: r.action, rewardTitle: r.rewardTitle, cost: r.cost })))
  }

  async function saveEdit() {
    if (!editingProfile) return
    if (!editName.trim()) {
      toast.error("Give the profile a name")
      return
    }
    if (editDrafts.length === 0) {
      toast.error("A profile needs at least one reward")
      return
    }
    setBusy("edit")
    try {
      await api.saveRewardProfile(editName.trim(), editColor, editDrafts, editingProfile.name)
      toast.success(`Updated "${editName.trim()}"`)
      setEditingProfile(null)
      refresh()
    } catch (e) {
      toast.error("Couldn't update profile", { description: String(e) })
    } finally {
      setBusy(null)
    }
  }

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-3">
        <div>
          <CardTitle>Reward profiles</CardTitle>
          <CardDescription>
            Click a profile to activate it: replaces whatever's live on Twitch with that set.
          </CardDescription>
        </div>
        <Dialog open={newOpen} onOpenChange={setNewOpen}>
          <DialogTrigger asChild>
            <Button size="sm" className="gap-1.5">
              <FolderPlus className="size-4" /> New profile
            </Button>
          </DialogTrigger>
          <DialogContent className="sm:max-w-xl">
            <DialogHeader>
              <DialogTitle>New reward profile</DialogTitle>
              <DialogDescription>
                Build a fresh set of rewards here without touching what's currently live on
                Twitch. Nothing is created until this profile is activated.
              </DialogDescription>
            </DialogHeader>
            <div className="grid gap-4 py-2">
              <div className="grid grid-cols-2 gap-4">
                <div className="grid gap-2">
                  <Label>Profile name</Label>
                  <Input
                    placeholder="Speedrun night"
                    value={newName}
                    onChange={(e) => setNewName(e.target.value)}
                  />
                </div>
                <div className="grid gap-2">
                  <Label>Color (optional)</Label>
                  <ColorPicker value={newColor} onChange={setNewColor} />
                </div>
              </div>
              <DraftRewardEditor drafts={newDrafts} setDrafts={setNewDrafts} />
            </div>
            <DialogFooter>
              <Button onClick={createNew} disabled={busy === "new"}>
                Create profile
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
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4">
            {profiles.map((p) => {
              const isActive = p.name === active
              const clickable = !isActive && busy === null
              return (
                <button
                  key={p.name}
                  type="button"
                  disabled={!clickable}
                  onClick={() => activate(p.name)}
                  className={`group relative flex flex-col gap-2 rounded-lg border p-3 text-left transition ${
                    p.color ? "" : "bg-card"
                  } ${clickable ? "cursor-pointer hover:brightness-110" : "cursor-default"} ${
                    busy === p.name ? "opacity-60" : ""
                  }`}
                  style={p.color ? { backgroundColor: p.color, borderColor: p.color, color: "#18181b" } : undefined}
                >
                  <div className="flex items-start justify-between gap-2">
                    {isActive ? (
                      <Badge className="gap-1 bg-black/15 text-inherit">
                        <Check className="size-3" /> Active
                      </Badge>
                    ) : (
                      <Badge
                        variant="secondary"
                        className={p.color ? "bg-black/10 text-inherit" : undefined}
                      >
                        {p.rewards.length} rewards
                      </Badge>
                    )}
                    <div className="flex items-center gap-0.5 opacity-0 transition-opacity group-hover:opacity-100">
                      <span
                        role="button"
                        tabIndex={0}
                        onClick={(e) => {
                          e.stopPropagation()
                          openEdit(p)
                        }}
                        className={`rounded p-1 ${p.color ? "hover:bg-black/10" : "text-muted-foreground hover:bg-foreground/10"}`}
                      >
                        <Pencil className="size-3.5" />
                      </span>
                      <span
                        role="button"
                        tabIndex={0}
                        onClick={(e) => {
                          e.stopPropagation()
                          remove(p.name)
                        }}
                        className={`rounded p-1 ${p.color ? "hover:bg-black/10" : "text-muted-foreground hover:bg-foreground/10"}`}
                      >
                        <Trash2 className="size-3.5" />
                      </span>
                    </div>
                  </div>
                  <div className="truncate text-sm font-medium" title={p.name}>
                    {p.name}
                  </div>
                </button>
              )
            })}
          </div>
        )}
      </CardContent>
      <Dialog open={editingProfile !== null} onOpenChange={(o) => !o && setEditingProfile(null)}>
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>Edit reward profile</DialogTitle>
            <DialogDescription>
              Changes here only affect this saved profile. Re-activate it to push them live on
              Twitch.
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 py-2">
            <div className="grid grid-cols-2 gap-4">
              <div className="grid gap-2">
                <Label>Profile name</Label>
                <Input value={editName} onChange={(e) => setEditName(e.target.value)} />
              </div>
              <div className="grid gap-2">
                <Label>Color (optional)</Label>
                <ColorPicker value={editColor} onChange={setEditColor} />
              </div>
            </div>
            <DraftRewardEditor drafts={editDrafts} setDrafts={setEditDrafts} />
          </div>
          <DialogFooter>
            <Button onClick={saveEdit} disabled={busy === "edit"}>
              Save changes
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  )
}

export function RewardsTab() {
  const { data: rewards, refresh } = usePolling<RewardAction[]>(() => api.rewards(), 15_000)
  const { data: profilesData, refresh: refreshProfiles } = usePolling<RewardProfilesResponse>(
    () => api.rewardProfiles(),
    15_000,
  )
  const [open, setOpen] = useState(false)
  const [action, setAction] = useState("")
  const [title, setTitle] = useState("")
  const [cost, setCost] = useState("500")
  const [saving, setSaving] = useState(false)
  const [saveOpen, setSaveOpen] = useState(false)
  const [saveName, setSaveName] = useState("")
  const [saveColor, setSaveColor] = useState("")
  const [savingProfile, setSavingProfile] = useState(false)

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

  async function saveCurrentAsProfile() {
    if (!saveName.trim()) {
      toast.error("Give the profile a name")
      return
    }
    setSavingProfile(true)
    try {
      const drafts: RewardDraft[] = (rewards ?? []).map((r) => ({
        action: r.action,
        rewardTitle: r.rewardTitle,
        cost: r.cost,
      }))
      await api.saveRewardProfile(saveName.trim(), saveColor, drafts)
      toast.success(`Saved current rewards as "${saveName.trim()}"`)
      setSaveOpen(false)
      setSaveName("")
      setSaveColor("")
      refreshProfiles()
    } catch (e) {
      toast.error("Couldn't save profile", { description: String(e) })
    } finally {
      setSavingProfile(false)
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <RewardProfilesCard
        profiles={profilesData?.profiles ?? []}
        active={profilesData?.active ?? ""}
        refresh={refreshProfiles}
        onActivated={refresh}
      />
      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <div>
            <CardTitle>Channel-points-only actions</CardTitle>
            <CardDescription>
              These only run when redeemed on Twitch. Typing them in chat is blocked for viewers
              (mods are exempt).
            </CardDescription>
          </div>
          <div className="flex gap-2">
            <Dialog open={saveOpen} onOpenChange={setSaveOpen}>
              <DialogTrigger asChild>
                <Button size="sm" variant="outline" className="gap-1.5" disabled={!rewards?.length}>
                  <FolderPlus className="size-4" /> Save as profile
                </Button>
              </DialogTrigger>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>Save current rewards as a profile</DialogTitle>
                  <DialogDescription>
                    Snapshots the channel-points-only actions below under this name.
                  </DialogDescription>
                </DialogHeader>
                <div className="grid gap-4 py-2">
                  <div className="grid gap-2">
                    <Label>Profile name</Label>
                    <Input
                      placeholder="Speedrun night"
                      value={saveName}
                      onChange={(e) => setSaveName(e.target.value)}
                    />
                  </div>
                  <div className="grid gap-2">
                    <Label>Color (optional)</Label>
                    <ColorPicker value={saveColor} onChange={setSaveColor} />
                  </div>
                </div>
                <DialogFooter>
                  <Button onClick={saveCurrentAsProfile} disabled={savingProfile}>
                    Save
                  </Button>
                </DialogFooter>
              </DialogContent>
            </Dialog>
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
                      Same syntax as a chat command, without the prefix. E.g. <code>lwin</code>,{" "}
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
          </div>
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
