import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Header } from "@/components/Header"
import { OverviewTab } from "@/components/OverviewTab"
import { LiveMonitorTab } from "@/components/LiveMonitorTab"
import { RewardsTab } from "@/components/RewardsTab"
import { SettingsTab } from "@/components/SettingsTab"
import { api } from "@/lib/api"
import { usePolling } from "@/hooks/usePolling"

export default function App() {
  const { data: status, refresh } = usePolling(() => api.status(), 3000)

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
