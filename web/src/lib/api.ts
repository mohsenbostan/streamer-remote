export interface StatusResponse {
  version: string
  localOnly: boolean
  twitchConfigured: boolean
  twitchConnected: boolean
  paused: boolean
  channel: string
}

export interface Blacklist {
  deniedKeys: string[]
  deniedCombos: string[][]
}

export interface Settings {
  prefix: string
  modOnlyMode: boolean
  textToSpeechEnabled: boolean
  globalCooldownMs: number
  perUserCooldownMs: number
  maxComboSize: number
  maxSequenceSteps: number
  tapHoldMs: number
  maxHoldMs: number
  maxMoveStep: number
  logDebug: boolean
  blacklist: Blacklist
}

export interface RewardAction {
  action: string
  rewardTitle: string
  cost: number
  rewardId: string
}

export interface RewardProfile {
  name: string
  rewards: RewardAction[]
}

export interface RewardProfilesResponse {
  profiles: RewardProfile[]
  active: string
}

export interface TwitchAuthState {
  state: "idle" | "pending" | "connected" | "error"
  verificationUri?: string
  userCode?: string
  error?: string
}

export interface UpdateInfo {
  current: string
  latest: string
  available: boolean
}

export type Permission = "everyone" | "subscriber" | "vip" | "moderator" | "broadcaster"

export interface LiveEvent {
  time: string
  level: "DEBUG" | "INFO" | "WARN" | "ERROR"
  msg: string
  attrs: Record<string, string>
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: { "Content-Type": "application/json", ...init?.headers },
  })
  if (!res.ok) {
    const body = await res.text()
    throw new Error(body || `${res.status} ${res.statusText}`)
  }
  const text = await res.text()
  if (!text) return undefined as T
  return JSON.parse(text) as T
}

export const api = {
  status: () => request<StatusResponse>("/api/status"),

  settings: () => request<Settings>("/api/settings"),
  updateSettings: (s: Settings) =>
    request<void>("/api/settings", { method: "PUT", body: JSON.stringify(s) }),

  setupTwitch: (channel: string, clientId: string) =>
    request<void>("/api/twitch/setup", {
      method: "POST",
      body: JSON.stringify({ channel, clientId }),
    }),
  connectTwitch: () => request<void>("/api/twitch/connect", { method: "POST" }),
  twitchAuthState: () => request<TwitchAuthState>("/api/twitch/auth"),
  logoutTwitch: () => request<void>("/api/twitch/logout", { method: "POST" }),

  pause: () => request<void>("/api/pause", { method: "POST" }),
  resume: () => request<void>("/api/resume", { method: "POST" }),

  test: (permission: Permission, text: string) =>
    request<void>("/api/test", { method: "POST", body: JSON.stringify({ permission, text }) }),

  rewards: () => request<RewardAction[]>("/api/rewards"),
  addReward: (action: string, rewardTitle: string, cost: number) =>
    request<RewardAction>("/api/rewards", {
      method: "POST",
      body: JSON.stringify({ action, rewardTitle, cost }),
    }),
  removeReward: (rewardId: string) =>
    request<void>(`/api/rewards/${encodeURIComponent(rewardId)}`, { method: "DELETE" }),

  rewardProfiles: () => request<RewardProfilesResponse>("/api/reward-profiles"),
  saveRewardProfile: (name: string) =>
    request<RewardProfile>("/api/reward-profiles", { method: "POST", body: JSON.stringify({ name }) }),
  deleteRewardProfile: (name: string) =>
    request<void>(`/api/reward-profiles/${encodeURIComponent(name)}`, { method: "DELETE" }),
  activateRewardProfile: (name: string) =>
    request<RewardProfilesResponse>(`/api/reward-profiles/${encodeURIComponent(name)}/activate`, {
      method: "POST",
    }),

  checkUpdate: () => request<UpdateInfo>("/api/update"),
  applyUpdate: () => request<void>("/api/update/apply", { method: "POST" }),
}

/** Subscribes to the live event feed. Returns an unsubscribe function. */
export function subscribeEvents(onEvent: (e: LiveEvent) => void): () => void {
  const source = new EventSource("/api/events")
  source.onmessage = (msg) => {
    try {
      onEvent(JSON.parse(msg.data))
    } catch {
      // ignore malformed frames
    }
  }
  return () => source.close()
}
