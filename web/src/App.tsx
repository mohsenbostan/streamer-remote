import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Header } from "@/components/Header"
import { OverviewTab } from "@/components/OverviewTab"
import { LiveMonitorTab } from "@/components/LiveMonitorTab"
import { RewardsTab } from "@/components/RewardsTab"
import { SettingsTab } from "@/components/SettingsTab"
import { api, subscribeEvents } from "@/lib/api"
import { usePolling } from "@/hooks/usePolling"
import { useEffect } from "react"

export default function App() {
  const { data: status, refresh } = usePolling(() => api.status(), 3000)
  const isLiveMonitorPopout = new URLSearchParams(window.location.search).get("popout") === "live-monitor"

  useEffect(() => {
    if (isLiveMonitorPopout) return
    return subscribeEvents((event) => {
      if (event.msg !== "text-to-speech") return
      speak(event.attrs.text)
    })
  }, [isLiveMonitorPopout])

  if (isLiveMonitorPopout) {
    return (
      <div className="min-h-screen bg-background p-6">
        <LiveMonitorTab popout />
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-background">
      <Header status={status} onChanged={refresh} />
      <main className="mx-auto max-w-3xl px-6 py-8">
        <Tabs defaultValue="overview">
          <TabsList>
            <TabsTrigger value="overview">Overview</TabsTrigger>
            <TabsTrigger value="live">Live monitor</TabsTrigger>
            <TabsTrigger value="rewards">Rewards</TabsTrigger>
            <TabsTrigger value="settings">Settings</TabsTrigger>
          </TabsList>
          <TabsContent value="overview" className="mt-6">
            <OverviewTab status={status} onChanged={refresh} />
          </TabsContent>
          <TabsContent value="live" className="mt-6">
            <LiveMonitorTab />
          </TabsContent>
          <TabsContent value="rewards" className="mt-6">
            <RewardsTab />
          </TabsContent>
          <TabsContent value="settings" className="mt-6">
            <SettingsTab status={status} onChanged={refresh} />
          </TabsContent>
        </Tabs>
      </main>
    </div>
  )
}

function speak(text?: string) {
  if (!text || !("speechSynthesis" in window)) return
  window.speechSynthesis.speak(new SpeechSynthesisUtterance(text))
}
