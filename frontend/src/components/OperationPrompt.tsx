import { useState } from "react"
import { Check, ChevronDown, Copy, FileText } from "lucide-react"
import { Button } from "@/components/ui/button"
import { agentName } from "@/components/RunTask"
import type { AgentProvider } from "@/lib/api"

export function OperationPrompt({ prompt, provider }: { prompt: string; provider: AgentProvider }) {
  const [copied, setCopied] = useState(false)
  const [error, setError] = useState(false)
  const [title, ...lines] = prompt.trim().split(/\r?\n/)
  const notes = lines.join("\n").trim()

  return (
    <details className="operation-prompt">
      <summary>
        <FileText aria-hidden="true" />
        <span className="operation-prompt-label"><strong>Original prompt</strong><span>Sent to {agentName(provider)}</span></span>
        <ChevronDown className="operation-prompt-chevron" aria-hidden="true" />
      </summary>
      <div className="operation-prompt-body">
        <div className="operation-prompt-toolbar">
          <span>Task instructions</span>
          <Button type="button" variant="ghost" size="sm" aria-label="Copy original prompt" onClick={async () => {
            try {
              await navigator.clipboard.writeText(prompt)
              setCopied(true)
              setError(false)
            } catch { setCopied(false); setError(true) }
          }}>{copied ? <Check /> : <Copy />}{copied ? "Copied" : "Copy prompt"}</Button>
        </div>
        {error && <p role="alert" className="operation-prompt-copy-error">Could not copy. Select the prompt text to copy it manually.</p>}
        <div className="operation-prompt-content" tabIndex={0} role="region" aria-label="Original prompt text">
          <div className="operation-prompt-document">
            <p className="operation-prompt-title">{title}</p>
            {notes && <p className="operation-prompt-notes">{notes}</p>}
          </div>
        </div>
      </div>
    </details>
  )
}
