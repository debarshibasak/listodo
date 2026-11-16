import { useEffect, useLayoutEffect, useRef, useState } from "react"
import { ArrowLeft, ArrowUp, ArrowUpRight, Check, CircleCheck, Command, Copy, Loader2, RefreshCw, Sparkles, Square, Terminal } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { request, relativeTime, type Operation } from "@/lib/api"
import { agentName } from "@/components/RunTask"
import { OperationPrompt } from "@/components/OperationPrompt"
import { usePageVisible } from "@/lib/use-page-visible"

const statusName = { running: "Running", succeeded: "Finished", failed: "Failed", cancelled: "Stopped", interrupted: "Interrupted" }

export function Operations({ operations, error, query, selected, onSelect, onRefresh, onTask, onContinued }: {
  operations: Operation[]
  error: string
  query: string
  selected: string | null
  onSelect: (id: string | null) => void
  onRefresh: () => Promise<void>
  onTask: (id: string) => void
  onContinued: (operation: Operation) => void
}) {
  const pageVisible = usePageVisible()
  const [filter, setFilter] = useState("all")
  const [operation, setOperation] = useState<Operation | null>(null)
  const [detailError, setDetailError] = useState("")
  const [pollError, setPollError] = useState("")
  const [stopping, setStopping] = useState(false)
  const [copied, setCopied] = useState(false)
  const stopRequest = useRef(0)
  const outputRef = useRef<HTMLPreElement>(null)
  const followOutput = useRef(true)
  const promptRef = useRef<HTMLInputElement>(null)
  const [prompt, setPrompt] = useState("")
  const [continuing, setContinuing] = useState(false)
  const [detailVersion, setDetailVersion] = useState(0)
  useEffect(() => {
    followOutput.current = true
    setOperation(null)
    setDetailError("")
    setPollError("")
    setStopping(false)
    setCopied(false)
    setPrompt("")
    setContinuing(false)
    return () => { stopRequest.current++ }
  }, [selected])
  useEffect(() => {
    if (!selected || !pageVisible) return
    let active = true
    let timer: ReturnType<typeof setTimeout>
    const poll = async () => {
      let running = true
      try {
        const result = await request<Operation>(`operations/${selected}`)
        if (!active) return
        setOperation(current => current?.id === result.id && current.updatedAt === result.updatedAt &&
          current.status === result.status && current.output === result.output && current.sessionId === result.sessionId &&
          current.turn === result.turn && current.error === result.error ? current : result)
        setPollError("")
        running = result.status === "running"
        if (!running) setStopping(false)
      } catch (e) {
        if (active) setPollError(e instanceof Error ? e.message : String(e))
      }
      if (active && running) timer = setTimeout(poll, stopping ? 200 : 1500)
    }
    void poll()
    return () => { active = false; clearTimeout(timer) }
  }, [selected, stopping, detailVersion, pageVisible])
  useLayoutEffect(() => {
    const output = outputRef.current
    if (output && followOutput.current) output.scrollTop = output.scrollHeight
  }, [operation?.id, operation?.output, operation?.status])
  useLayoutEffect(() => {
    if (operation?.status !== "running" && followOutput.current) promptRef.current?.focus({ preventScroll: true })
  }, [operation?.id, operation?.status])

  async function continueSession(event: React.FormEvent) {
    event.preventDefault()
    if (!operation || operation.status === "running" || !operation.sessionId || continuing || !prompt.trim()) return
    const requestId = ++stopRequest.current
    setContinuing(true)
    setDetailError("")
    try {
      const result = await request<Operation>(`operations/${operation.id}/continue`, { prompt: prompt.trim(), turn: operation.turn || 1 })
      if (requestId !== stopRequest.current) return
      followOutput.current = true
      setOperation(result)
      setPrompt("")
      setDetailVersion(current => current + 1)
      onContinued(result)
    } catch (e) {
      if (requestId === stopRequest.current) setDetailError(e instanceof Error ? e.message : String(e))
    } finally {
      if (requestId === stopRequest.current) setContinuing(false)
    }
  }

  async function stopRun() {
    if (!operation || operation.status !== "running" || stopping) return
    const requestId = ++stopRequest.current
    setStopping(true)
    setDetailError("")
    try {
      await request(`operations/${operation.id}/cancel`, {})
      await onRefresh()
    } catch (e) {
      if (requestId !== stopRequest.current) return
      setDetailError(e instanceof Error ? e.message : String(e))
      setStopping(false)
    }
  }

  const visible = operations.filter(op =>
    (filter === "all" || (filter === "running" ? op.status === "running" : op.status !== "running")) &&
    (!query || `${op.title} ${op.provider} ${op.directory} ${op.sessionId}`.toLowerCase().includes(query.toLowerCase())),
  )

  if (selected) return (
    <section className="operation-detail">
      <div className="operation-detail-toolbar">
        <Button variant="ghost" size="sm" onClick={() => onSelect(null)}><ArrowLeft />All operations</Button>
        <Button variant="ghost" size="sm" disabled={!operation} onClick={() => operation && onTask(operation.taskId)}>View task<ArrowUpRight /></Button>
      </div>
      {(detailError || pollError) && <p role="alert" className="operation-error">{detailError || pollError}</p>}
      {!operation ? <div className="operations-loading"><Loader2 className="animate-spin" />Loading session…</div> : <>
        <div className="operation-detail-heading">
          <div><h2>{operation.title}</h2><p>{agentName(operation.provider)} · {new Date(operation.startedAt).toLocaleString()}</p></div>
          <span className={`operation-status ${operation.status}`} role="status">{operation.status === "running" && <Loader2 className="animate-spin" />}{operation.status === "running" && stopping ? "Stopping…" : statusName[operation.status]}</span>
        </div>
        <div className="operation-metadata">
          <div><span>Folder</span><code>{operation.directory}</code></div>
          {operation.sessionId && <div><span>Session</span><code>{operation.sessionId}</code><Button variant="ghost" size="icon-xs" aria-label="Copy session ID" onClick={async () => {
            try { await navigator.clipboard.writeText(operation.sessionId); setCopied(true) }
            catch { setDetailError("Could not copy the session ID. Select it to copy manually.") }
          }}>{copied ? <Check /> : <Copy />}</Button></div>}
        </div>
        {operation.error && <p role="alert" className="operation-error">{operation.error}</p>}
        <div className="operation-output-heading">
          <h3>Session output</h3>
          {(operation.status === "running" || operation.status === "cancelled") && <Button variant="outline" size="sm"
            className="operation-stop-button" disabled={stopping || operation.status === "cancelled"}
            aria-busy={stopping} onClick={() => void stopRun()}>
            {operation.status === "cancelled" ? <Check /> : stopping ? <Loader2 className="animate-spin" /> : <Square />}
            {operation.status === "cancelled" ? "Stopped" : stopping ? "Stopping…" : "Stop run"}
          </Button>}
        </div>
        <div className="operation-console">
          <pre ref={outputRef} className="operation-output" tabIndex={0} aria-label="Session output" onScroll={event => {
            const output = event.currentTarget
            followOutput.current = output.scrollHeight - output.clientHeight - output.scrollTop < 24
          }}>{operation.output || (operation.status === "running" ? "Waiting for agent output…" : "No output was recorded.")}</pre>
          {operation.status !== "running" && <form className="operation-followup" onSubmit={continueSession} aria-busy={continuing}>
            <span aria-hidden="true">&gt;</span>
            <Input ref={promptRef} aria-label="Continue agent session" aria-describedby={!operation.sessionId ? "operation-session-unavailable" : undefined}
              autoComplete="off" spellCheck={false} maxLength={30000} value={prompt} disabled={continuing || !operation.sessionId}
              onChange={event => setPrompt(event.target.value)} />
            <Button type="submit" size="icon-xs" variant="ghost" aria-label="Send prompt" disabled={continuing || !operation.sessionId || !prompt.trim()}>
              {continuing ? <Loader2 className="animate-spin" /> : <ArrowUp />}
            </Button>
          </form>}
        </div>
        {operation.status !== "running" && !operation.sessionId && <p id="operation-session-unavailable" className="operation-review-note">No session was recorded. Start a new run from the task.</p>}
        {operation.status === "succeeded" && <div className="operation-review">
          <CircleCheck aria-hidden="true" />
          <div><h3>Ready for review</h3><p>Check the changes before marking this task complete.</p></div>
          <Button variant="outline" size="sm" onClick={() => onTask(operation.taskId)}>View task<ArrowUpRight /></Button>
        </div>}
        {operation.prompt && <OperationPrompt key={operation.id} prompt={operation.prompt} provider={operation.provider} />}
      </>}
    </section>
  )
  return (
    <section className="operations-view">
      <div className="task-list-toolbar">
        <Tabs value={filter} onValueChange={setFilter}><TabsList>
          <TabsTrigger value="all">All runs<span>{operations.length}</span></TabsTrigger>
          <TabsTrigger value="running">Running</TabsTrigger>
          <TabsTrigger value="finished">Finished</TabsTrigger>
        </TabsList></Tabs>
        <Button variant="ghost" size="sm" aria-label="Refresh operations" onClick={() => void onRefresh()}><RefreshCw className="size-3.5" /></Button>
      </div>
      {error && <p role="alert" className="operation-error">{error}</p>}
      {!visible.length ? <div className="empty-state">
        <Terminal className="operations-empty-icon" />
        <h2>{query || filter !== "all" ? "No matching operations" : "No operations yet"}</h2>
        <p>Choose “Run with agent” from a task’s menu to start a session.</p>
      </div> : <div className="operation-list">{visible.map(op => <button className="operation-row" key={op.id} onClick={() => onSelect(op.id)}>
        <span className="harness-icon">{op.provider === "codex" ? <Command /> : <Sparkles />}</span>
        <span className="operation-row-info"><strong>{op.title}</strong><span>{agentName(op.provider)}<i>·</i>{op.directory}</span></span>
        <span className={`operation-status ${op.status}`}>{op.status === "running" && <Loader2 className="animate-spin" />}{statusName[op.status]}</span>
        <time dateTime={op.startedAt}>{relativeTime(op.startedAt)}</time>
        <ArrowUpRight className="operation-open" />
      </button>)}</div>}
    </section>
  )
}
