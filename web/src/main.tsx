import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import "./index.css"
import App from "./App.tsx"
import { Toaster } from "@/components/ui/sonner"
import { TooltipProvider } from "@/components/ui/tooltip"
import { ThemeProvider } from "next-themes"

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ThemeProvider attribute="class" defaultTheme="system" enableSystem>
      <TooltipProvider delayDuration={300}>
        <App />
        <Toaster position="bottom-right" />
      </TooltipProvider>
    </ThemeProvider>
  </StrictMode>,
)
